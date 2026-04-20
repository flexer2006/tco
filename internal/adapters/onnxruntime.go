package adapters

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

var (
	environmentInitErr  error
	environmentInitOnce sync.Once
)

type (
	NativeRuntimeOption func(*NativeRuntime)
	NativeRuntime       struct {
		sharedLibraryPath, inputName, outputName string
	}
)

func WithNativeSharedLibraryPath(path string) NativeRuntimeOption {
	return func(runtime *NativeRuntime) {
		runtime.sharedLibraryPath = strings.TrimSpace(path)
	}
}

func WithNativeInputOutputNames(inputName, outputName string) NativeRuntimeOption {
	return func(runtime *NativeRuntime) {
		runtime.inputName = strings.TrimSpace(inputName)
		runtime.outputName = strings.TrimSpace(outputName)
	}
}

func NewNativeRuntime(options ...NativeRuntimeOption) *NativeRuntime {
	runtime := &NativeRuntime{}
	for _, option := range options {
		if option != nil {
			option(runtime)
		}
	}
	return runtime
}

func (r *NativeRuntime) Encode(ctx context.Context, modelPath string, texts []string) ([][]float32, error) {
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
	inputName, outputName, err := r.resolveInputOutputNames(modelPath)
	if err != nil {
		return nil, err
	}
	inputTensor, err := ort.NewStringTensor(ort.NewShape(int64(len(texts))))
	if err != nil {
		return nil, fmt.Errorf("create input tensor: %w", err)
	}
	defer func() {
		_ = inputTensor.Destroy()
	}()
	if err := inputTensor.SetContents(texts); err != nil {
		return nil, fmt.Errorf("set input tensor contents: %w", err)
	}
	session, err := ort.NewDynamicAdvancedSession(modelPath, []string{inputName}, []string{outputName}, nil)
	if err != nil {
		return nil, fmt.Errorf("create dynamic session: %w", err)
	}
	defer func() {
		_ = session.Destroy()
	}()
	outputs := []ort.Value{nil}
	if err := session.Run([]ort.Value{inputTensor}, outputs); err != nil {
		return nil, fmt.Errorf("run dynamic session: %w", err)
	}
	if outputs[0] == nil {
		return nil, errors.New("runtime returned nil output")
	}
	defer func() {
		_ = outputs[0].Destroy()
	}()
	floatOutput, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, fmt.Errorf("runtime returned unexpected output type %T", outputs[0])
	}
	data := floatOutput.GetData()
	if len(data)%len(texts) != 0 {
		return nil, fmt.Errorf("unexpected output data size %d for batch %d", len(data), len(texts))
	}
	dimension := len(data) / len(texts)
	result := make([][]float32, 0, len(texts))
	for i := range len(texts) {
		start := i * dimension
		result = append(result, slices.Clone(data[start:start+dimension]))
	}
	return result, nil
}

func (r *NativeRuntime) initializeEnvironment() error {
	environmentInitOnce.Do(func() {
		sharedLibraryPath := r.sharedLibraryPath
		if sharedLibraryPath == "" {
			sharedLibraryPath = strings.TrimSpace(os.Getenv("ONNXRUNTIME_SHARED_LIBRARY"))
		}
		if sharedLibraryPath != "" {
			ort.SetSharedLibraryPath(sharedLibraryPath)
		}
		environmentInitErr = ort.InitializeEnvironment()
	})
	if environmentInitErr != nil {
		return fmt.Errorf("initialize onnxruntime environment: %w", environmentInitErr)
	}
	return nil
}

func (r *NativeRuntime) resolveInputOutputNames(modelPath string) (string, string, error) {
	inputName, outputName := r.inputName, r.outputName
	if inputName != "" && outputName != "" {
		return inputName, outputName, nil
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
			"model input contract mismatch: expected exactly 1 tensor(string) input, got %d inputs (%s)",
			len(inputs),
			formatInputOutputInfos(inputs),
		)
	}
	if inputs[0].OrtValueType != ort.ONNXTypeTensor || inputs[0].DataType != ort.TensorElementDataTypeString {
		return "", "", fmt.Errorf(
			"model input contract mismatch: expected tensor(string) input, got %s",
			inputs[0].String(),
		)
	}
	if outputs[0].OrtValueType != ort.ONNXTypeTensor || outputs[0].DataType != ort.TensorElementDataTypeFloat {
		return "", "", fmt.Errorf(
			"model output contract mismatch: expected first output tensor(float), got %s",
			outputs[0].String(),
		)
	}
	if inputName == "" {
		inputName = strings.TrimSpace(inputs[0].Name)
	}
	if outputName == "" {
		outputName = strings.TrimSpace(outputs[0].Name)
	}
	if inputName == "" || outputName == "" {
		return "", "", errors.New("model input or output name is empty")
	}
	return inputName, outputName, nil
}

func formatInputOutputInfos(values []ort.InputOutputInfo) string {
	if len(values) == 0 {
		return ""
	}
	descriptions := make([]string, 0, len(values))
	for _, value := range values {
		descriptions = append(descriptions, value.String())
	}
	return strings.Join(descriptions, "; ")
}
