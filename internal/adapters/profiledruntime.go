package adapters

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

var (
	profiledEnvironmentInitErr  error
	profiledEnvironmentInitOnce sync.Once
)

type (
	ProfiledRuntime struct {
		profile                                                                      ModelProfile
		sharedLibraryPath, tokenizerPath, inputIDName, attentionMaskName, outputName string
		tokenizer                                                                    *Tokenizer
		cacheMu                                                                      sync.RWMutex
		stringIOCacheByMod                                                           map[string]stringIONames
		bertIOCacheByMod                                                             map[string]bertIONames
	}
	stringIONames struct {
		input, output string
	}
	bertIONames struct {
		inputID, attentionMask, tokenType, output string
	}
	ProfiledRuntimeOption func(*ProfiledRuntime)
)

func WithProfiledSharedLibraryPath(path string) ProfiledRuntimeOption {
	return func(r *ProfiledRuntime) { r.sharedLibraryPath = strings.TrimSpace(path) }
}

func WithProfiledTokenizer(path string, tokenizer *Tokenizer) ProfiledRuntimeOption {
	return func(r *ProfiledRuntime) {
		r.tokenizer = tokenizer
		r.tokenizerPath = path
	}
}

func NewProfiledRuntime(profile ModelProfile, options ...ProfiledRuntimeOption) *ProfiledRuntime {
	r := &ProfiledRuntime{
		profile:            profile,
		stringIOCacheByMod: make(map[string]stringIONames),
		bertIOCacheByMod:   make(map[string]bertIONames),
	}
	for _, opt := range options {
		if opt != nil {
			opt(r)
		}
	}
	return r
}

func (r *ProfiledRuntime) Encode(ctx context.Context, modelPath string, texts []string) ([][]float32, error) {
	if ctx == nil {
		return nil, errors.New("context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(modelPath) == "" {
		return nil, errors.New("model path must not be empty")
	}
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	if err := r.initializeEnvironment(); err != nil {
		return nil, err
	}
	switch r.profile {
	case ProfileBertTokenizedMeanPooling:
		return r.encodeBertTokenized(ctx, modelPath, texts)
	case ProfileStringInputDirect:
		return r.encodeStringDirect(ctx, modelPath, texts)
	default:
		return nil, fmt.Errorf("unsupported model profile: %q", r.profile)
	}
}

func (r *ProfiledRuntime) initializeEnvironment() error {
	profiledEnvironmentInitOnce.Do(func() {
		sharedLibraryPath := r.sharedLibraryPath
		if sharedLibraryPath == "" {
			sharedLibraryPath = strings.TrimSpace(os.Getenv("ONNXRUNTIME_SHARED_LIBRARY"))
		}
		if sharedLibraryPath != "" {
			ort.SetSharedLibraryPath(sharedLibraryPath)
		}
		profiledEnvironmentInitErr = ort.InitializeEnvironment()
	})
	if profiledEnvironmentInitErr != nil {
		return fmt.Errorf("initialize onnxruntime environment: %w", profiledEnvironmentInitErr)
	}
	return nil
}

func (r *ProfiledRuntime) encodeStringDirect(ctx context.Context, modelPath string, texts []string) ([][]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	inputName, outputName, err := r.resolveStringInputOutput(modelPath)
	if err != nil {
		return nil, err
	}
	inputTensor, err := ort.NewStringTensor(ort.NewShape(int64(len(texts))))
	if err != nil {
		return nil, fmt.Errorf("create input tensor: %w", err)
	}
	defer func() { _ = inputTensor.Destroy() }()
	if err := inputTensor.SetContents(texts); err != nil {
		return nil, fmt.Errorf("set input tensor contents: %w", err)
	}
	data, _, err := runDynamicSessionFloatOutput(ctx, modelPath, []string{inputName}, outputName, []ort.Value{inputTensor})
	if err != nil {
		return nil, err
	}
	if len(data)%len(texts) != 0 {
		return nil, fmt.Errorf("unexpected output data size %d for batch %d", len(data), len(texts))
	}
	dimension := len(data) / len(texts)
	result := make([][]float32, len(texts))
	for i := range len(texts) {
		start := i * dimension
		result[i] = slices.Clone(data[start : start+dimension])
	}
	return result, nil
}

func (r *ProfiledRuntime) encodeBertTokenized(ctx context.Context, modelPath string, texts []string) ([][]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.tokenizer == nil {
		return nil, errors.New("bert_tokenized_mean_pooling profile requires a tokenizer")
	}
	encoded, err := r.tokenizer.encodeBatch(texts)
	if err != nil {
		return nil, fmt.Errorf("tokenize texts: %w", err)
	}
	maxSeqLen := 0
	for _, enc := range encoded {
		maxSeqLen = max(maxSeqLen, len(enc.InputIDs))
	}
	if maxSeqLen == 0 {
		return nil, errors.New("tokenizer produced empty sequences for all texts")
	}
	batchSize, seqLen := int64(len(texts)), int64(maxSeqLen)
	inputIDName, maskName, tokenTypeName, outputName, err := r.resolveBertInputOutputNames(modelPath)
	if err != nil {
		return nil, err
	}
	includeTokenTypes := tokenTypeName != ""
	inputIDsData, attentionMaskData, tokenTypeIDsData := buildBertInputData(encoded, maxSeqLen, includeTokenTypes)
	inputIDsTensor, err := ort.NewTensor(ort.NewShape(batchSize, seqLen), inputIDsData)
	if err != nil {
		return nil, fmt.Errorf("create input_ids tensor: %w", err)
	}
	defer func() { _ = inputIDsTensor.Destroy() }()
	maskTensor, err := ort.NewTensor(ort.NewShape(batchSize, seqLen), attentionMaskData)
	if err != nil {
		return nil, fmt.Errorf("create attention_mask tensor: %w", err)
	}
	defer func() { _ = maskTensor.Destroy() }()
	inputNames := []string{inputIDName, maskName}
	inputValues := []ort.Value{inputIDsTensor, maskTensor}
	if includeTokenTypes {
		tokenTypeTensor, err := ort.NewTensor(ort.NewShape(batchSize, seqLen), tokenTypeIDsData)
		if err != nil {
			return nil, fmt.Errorf("create token_type_ids tensor: %w", err)
		}
		defer func() { _ = tokenTypeTensor.Destroy() }()
		inputNames = append(inputNames, tokenTypeName)
		inputValues = append(inputValues, tokenTypeTensor)
	}
	data, shape, err := runDynamicSessionFloatOutput(ctx, modelPath, inputNames, outputName, inputValues)
	if err != nil {
		return nil, err
	}
	result, err := decodeBertOutputVectors(data, shape, len(texts), attentionMaskData)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func buildBertInputData(encoded []encodeResult, maxSeqLen int, includeTokenTypes bool) ([]int64, []int64, []int64) {
	flatLen := len(encoded) * maxSeqLen
	inputIDsData := make([]int64, flatLen)
	attentionMaskData := make([]int64, flatLen)
	var tokenTypeIDsData []int64
	if includeTokenTypes {
		tokenTypeIDsData = make([]int64, flatLen)
	}
	for row, enc := range encoded {
		base := row * maxSeqLen
		inputCount := min(len(enc.InputIDs), maxSeqLen)
		copy(inputIDsData[base:base+inputCount], enc.InputIDs[:inputCount])
		maskCount := min(len(enc.AttentionMask), maxSeqLen)
		copy(attentionMaskData[base:base+maskCount], enc.AttentionMask[:maskCount])
		if includeTokenTypes {
			tokenTypeCount := min(len(enc.TokenTypeIDs), maxSeqLen)
			if tokenTypeCount > 0 {
				copy(tokenTypeIDsData[base:base+tokenTypeCount], enc.TokenTypeIDs[:tokenTypeCount])
			}
		}
	}
	return inputIDsData, attentionMaskData, tokenTypeIDsData
}

func runDynamicSessionFloatOutput(ctx context.Context, modelPath string, inputNames []string, outputName string, inputValues []ort.Value) ([]float32, []int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	session, err := ort.NewDynamicAdvancedSession(modelPath, inputNames, []string{outputName}, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("create dynamic session: %w", err)
	}
	defer func() { _ = session.Destroy() }()
	outputs := []ort.Value{nil}
	if err := session.Run(inputValues, outputs); err != nil {
		return nil, nil, fmt.Errorf("run dynamic session: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if outputs[0] == nil {
		return nil, nil, errors.New("runtime returned nil output")
	}
	defer func() { _ = outputs[0].Destroy() }()
	floatOutput, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, nil, fmt.Errorf("runtime returned unexpected output type %T", outputs[0])
	}
	data, shape := slices.Clone(floatOutput.GetData()), slices.Clone([]int64(floatOutput.GetShape()))
	return data, shape, nil
}

func decodeBertOutputVectors(data []float32, shape []int64, batchSize int, attentionMask []int64) ([][]float32, error) {
	if len(data) == 0 {
		return nil, errors.New("runtime returned empty output")
	}
	if batchSize <= 0 {
		return nil, errors.New("batch size must be greater than 0")
	}
	switch len(shape) {
	case 2:
		return decodeSentenceEmbeddingOutput(data, shape, batchSize)
	case 3:
		return decodeLastHiddenStateOutput(data, shape, batchSize, attentionMask)
	default:
		return nil, fmt.Errorf("unexpected bert output rank %d (shape=%v): expected 2D sentence_embedding or 3D last_hidden_state", len(shape), shape)
	}
}

func decodeSentenceEmbeddingOutput(data []float32, shape []int64, batchSize int) ([][]float32, error) {
	rows, columns := int(shape[0]), int(shape[1])
	if rows <= 0 || columns <= 0 {
		return nil, fmt.Errorf("invalid sentence_embedding shape %v", shape)
	}
	if rows != batchSize {
		return nil, fmt.Errorf("output batch dimension %d does not match input batch size %d", rows, batchSize)
	}
	if rows*columns != len(data) {
		return nil, fmt.Errorf("unexpected sentence_embedding data size %d for shape %v", len(data), shape)
	}
	result := make([][]float32, rows)
	for i := range rows {
		start := i * columns
		vector := slices.Clone(data[start : start+columns])
		l2NormalizeInPlace(vector)
		result[i] = vector
	}
	return result, nil
}

func decodeLastHiddenStateOutput(data []float32, shape []int64, batchSize int, attentionMask []int64) ([][]float32, error) {
	rows, seqLen, hiddenSize := int(shape[0]), int(shape[1]), int(shape[2])
	if rows <= 0 || seqLen <= 0 || hiddenSize <= 0 {
		return nil, fmt.Errorf("invalid last_hidden_state shape %v", shape)
	}
	if rows != batchSize {
		return nil, fmt.Errorf("output batch dimension %d does not match input batch size %d", rows, batchSize)
	}
	if rows*seqLen*hiddenSize != len(data) {
		return nil, fmt.Errorf("unexpected last_hidden_state data size %d for shape %v", len(data), shape)
	}
	if len(attentionMask) < rows*seqLen {
		return nil, fmt.Errorf("attention mask length %d is smaller than required %d", len(attentionMask), rows*seqLen)
	}
	result := make([][]float32, rows)
	for i := range rows {
		vector := meanPoolForSequence(data, i, seqLen, hiddenSize, attentionMask)
		result[i] = vector
	}
	return result, nil
}

func meanPoolForSequence(data []float32, batchIdx int, seqLen, hiddenSize int, attentionMask []int64) []float32 {
	vec, maskSum := make([]float32, hiddenSize), 0.0
	for s := range seqLen {
		maskIdx := batchIdx*seqLen + s
		if maskIdx >= len(attentionMask) || attentionMask[maskIdx] == 0 {
			continue
		}
		maskSum++
		for h := range hiddenSize {
			vec[h] += data[(batchIdx*seqLen+s)*hiddenSize+h]
		}
	}
	if maskSum > 0 {
		for i := range vec {
			vec[i] /= float32(maskSum)
		}
	}
	l2NormalizeInPlace(vec)
	return vec
}

func l2NormalizeInPlace(vec []float32) {
	norm := float32(0)
	for _, v := range vec {
		norm += v * v
	}
	if norm = float32(math.Sqrt(float64(norm))); norm > 0 {
		for i := range vec {
			vec[i] /= norm
		}
	}
}

func (r *ProfiledRuntime) resolveStringInputOutput(modelPath string) (string, string, error) {
	if cached, ok := r.cachedStringIONames(modelPath); ok {
		return cached.input, cached.output, nil
	}
	inputs, outputs, err := ort.GetInputOutputInfo(modelPath)
	if err != nil {
		return "", "", fmt.Errorf("read model input/output metadata: %w", err)
	}
	if len(inputs) == 0 || len(outputs) == 0 {
		return "", "", errors.New("model has no input or output tensors")
	}
	if len(inputs) != 1 {
		return "", "", fmt.Errorf(
			"model input contract mismatch for string_input_direct: expected exactly 1 tensor(string) input, got %d inputs (%s)",
			len(inputs), formatInputOutputInfos(inputs),
		)
	}
	if inputs[0].OrtValueType != ort.ONNXTypeTensor || inputs[0].DataType != ort.TensorElementDataTypeString {
		return "", "", fmt.Errorf(
			"model input contract mismatch for string_input_direct: expected tensor(string) input, got %s",
			inputs[0].String(),
		)
	}
	if outputs[0].OrtValueType != ort.ONNXTypeTensor || outputs[0].DataType != ort.TensorElementDataTypeFloat {
		return "", "", fmt.Errorf(
			"model output contract mismatch for string_input_direct: expected first output tensor(float), got %s",
			outputs[0].String(),
		)
	}
	names := stringIONames{
		input:  strings.TrimSpace(inputs[0].Name),
		output: strings.TrimSpace(outputs[0].Name),
	}
	r.storeStringIONames(modelPath, names)
	return names.input, names.output, nil
}

func (r *ProfiledRuntime) resolveBertInputOutputNames(modelPath string) (inputID, attentionMask, tokenType, output string, err error) {
	if cached, ok := r.cachedBertIONames(modelPath); ok {
		return cached.inputID, cached.attentionMask, cached.tokenType, cached.output, nil
	}
	inputs, outputs, err := ort.GetInputOutputInfo(modelPath)
	if err != nil {
		return "", "", "", "", fmt.Errorf("read model input/output metadata: %w", err)
	}
	if len(inputs) == 0 || len(outputs) == 0 {
		return "", "", "", "", errors.New("model has no input or output tensors")
	}
	nameToInfo := mapInputOutputInfoByName(inputs)
	inputIDName, err := resolveInputIDName(nameToInfo, r.inputIDName)
	if err != nil {
		return "", "", "", "", err
	}
	maskName, err := resolveAttentionMaskName(nameToInfo, r.attentionMaskName, inputIDName)
	if err != nil {
		return "", "", "", "", err
	}
	tokenTypeName := resolveTokenTypeName(nameToInfo)
	outputName, err := resolveOutputName(outputs, r.outputName)
	if err != nil {
		return "", "", "", "", err
	}
	if inputIDName == "" || maskName == "" || outputName == "" {
		return "", "", "", "", errors.New("model input or output name is empty after resolution")
	}
	r.storeBertIONames(modelPath, bertIONames{
		inputID:       inputIDName,
		attentionMask: maskName,
		tokenType:     tokenTypeName,
		output:        outputName,
	})
	return inputIDName, maskName, tokenTypeName, outputName, nil
}

func mapInputOutputInfoByName(inputs []ort.InputOutputInfo) map[string]ort.InputOutputInfo {
	nameToInfo := make(map[string]ort.InputOutputInfo, len(inputs))
	for _, inp := range inputs {
		nameToInfo[strings.TrimSpace(inp.Name)] = inp
	}
	return nameToInfo
}

func resolveInputIDName(nameToInfo map[string]ort.InputOutputInfo, configured string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	if _, ok := nameToInfo["input_ids"]; ok {
		return "input_ids", nil
	}
	if inferred, ok := findFirstInt64Input(nameToInfo); ok {
		return inferred, nil
	}
	return "", errors.New("cannot resolve input_ids tensor name from model metadata")
}

func resolveAttentionMaskName(nameToInfo map[string]ort.InputOutputInfo, configured, inputIDName string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	if _, ok := nameToInfo["attention_mask"]; ok {
		return "attention_mask", nil
	}
	if inferred, ok := findSecondInt64Input(nameToInfo, inputIDName); ok {
		return inferred, nil
	}
	return "", errors.New("cannot resolve attention_mask tensor name from model metadata")
}

func resolveTokenTypeName(nameToInfo map[string]ort.InputOutputInfo) string {
	if _, ok := nameToInfo["token_type_ids"]; ok {
		return "token_type_ids"
	}
	if _, ok := nameToInfo["token_type_ids:0"]; ok {
		return "token_type_ids:0"
	}
	return ""
}

func resolveOutputName(outputs []ort.InputOutputInfo, configured string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	if hasOutputByName(outputs, "sentence_embedding") {
		return "sentence_embedding", nil
	}
	if hasOutputByName(outputs, "last_hidden_state") {
		return "last_hidden_state", nil
	}
	if len(outputs) > 0 {
		return strings.TrimSpace(outputs[0].Name), nil
	}
	return "", errors.New("cannot resolve output tensor name from model metadata")
}

func (r *ProfiledRuntime) cachedStringIONames(modelPath string) (stringIONames, bool) {
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()
	names, ok := r.stringIOCacheByMod[modelPath]
	return names, ok
}

func (r *ProfiledRuntime) storeStringIONames(modelPath string, names stringIONames) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	if r.stringIOCacheByMod == nil {
		r.stringIOCacheByMod = make(map[string]stringIONames)
	}
	r.stringIOCacheByMod[modelPath] = names
}

func (r *ProfiledRuntime) cachedBertIONames(modelPath string) (bertIONames, bool) {
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()
	names, ok := r.bertIOCacheByMod[modelPath]
	return names, ok
}

func (r *ProfiledRuntime) storeBertIONames(modelPath string, names bertIONames) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	if r.bertIOCacheByMod == nil {
		r.bertIOCacheByMod = make(map[string]bertIONames)
	}
	r.bertIOCacheByMod[modelPath] = names
}

func findFirstInt64Input(inputs map[string]ort.InputOutputInfo) (string, bool) {
	for _, info := range inputs {
		if info.OrtValueType == ort.ONNXTypeTensor && info.DataType == ort.TensorElementDataTypeInt64 {
			return info.Name, true
		}
	}
	return "", false
}

func findSecondInt64Input(inputs map[string]ort.InputOutputInfo, excludeName string) (string, bool) {
	for name, info := range inputs {
		if name != excludeName &&
			info.OrtValueType == ort.ONNXTypeTensor &&
			info.DataType == ort.TensorElementDataTypeInt64 {
			return info.Name, true
		}
	}
	return "", false
}

func hasOutputByName(outputs []ort.InputOutputInfo, name string) bool {
	for _, out := range outputs {
		if strings.TrimSpace(out.Name) == name {
			return true
		}
	}
	return false
}
