package embedding

import "errors"

var (
	ErrModelIDEmpty             = errors.New("model_id: must not be empty")
	ErrModelPathEmpty           = errors.New("model_path: must not be empty")
	ErrModelPathNotExist        = errors.New("model_path: file does not exist")
	ErrModelPathIsDirectory     = errors.New("model_path: path is a directory, expected an ONNX model file")
	ErrModelPathEmptyFile       = errors.New("model_path: file is empty (0 bytes)")
	ErrVectorDimensionInvalid   = errors.New("vector_dimension: must be greater than 0")
	ErrRuntimeNil               = errors.New("runtime must not be nil")
	ErrONNXEncoderNil           = errors.New("onnx encoder must not be nil")
	ErrContextNil               = errors.New("context must not be nil")
	ErrRuntimeVectorCount       = errors.New("runtime returned unexpected vector count")
	ErrRuntimeVectorDimension   = errors.New("runtime returned unexpected vector dimension")
	ErrModelHashEmpty           = errors.New("model_hash: must not be empty")
	ErrUnsupportedNormRule      = errors.New("normalization_rule: unsupported value")
	ErrModelProfileEmpty        = errors.New("model_profile: must not be empty")
	ErrUnsupportedModelProfile  = errors.New("model_profile: unsupported value")
	ErrModelProfileEmptyAlt     = errors.New("model profile: must not be empty")
	ErrModelPathMustNotBeEmpty  = errors.New("model path must not be empty")
	ErrUnexpectedOutputDataSize = errors.New("unexpected output data size")
	ErrBertTokenizerRequired    = errors.New("bert_tokenized_mean_pooling profile requires a tokenizer")
	ErrTokenizerEmptySequences  = errors.New("tokenizer produced empty sequences for all texts")
	ErrRuntimeNilOutput         = errors.New("runtime returned nil output")
	ErrUnexpectedOutputType     = errors.New("runtime returned unexpected output type")
	ErrRuntimeEmptyOutput       = errors.New("runtime returned empty output")
	ErrBatchSizeNotPositive     = errors.New("batch size must be greater than 0")
	ErrUnexpectedBertOutputRank = errors.New(
		"unexpected bert output rank: expected 2D sentence_embedding or 3D last_hidden_state",
	)
	ErrInvalidSentenceEmbeddingShape = errors.New("invalid sentence_embedding shape")
	ErrOutputBatchDimensionMismatch  = errors.New(
		"output batch dimension does not match input batch size",
	)
	ErrUnexpectedSentenceEmbDataSize = errors.New("unexpected sentence_embedding data size")
	ErrInvalidLastHiddenStateShape   = errors.New("invalid last_hidden_state shape")
	ErrUnexpectedLastHiddenDataSize  = errors.New("unexpected last_hidden_state data size")
	ErrAttentionMaskTooShort         = errors.New("attention mask length is smaller than required")
	ErrModelNoTensors                = errors.New("model has no input or output tensors")
	ErrStringInputCountMismatch      = errors.New(
		"model input contract mismatch for string_input_direct: expected exactly 1 tensor(string) input",
	)
	ErrStringInputTypeMismatch = errors.New(
		"model input contract mismatch for string_input_direct: expected tensor(string) input",
	)
	ErrStringOutputTypeMismatch = errors.New(
		"model output contract mismatch for string_input_direct: expected first output tensor(float)",
	)
	ErrEmptyResolvedIONames         = errors.New("model input or output name is empty after resolution")
	ErrCannotResolveInputIDs        = errors.New("cannot resolve input_ids tensor name from model metadata")
	ErrCannotResolveAttentionMask   = errors.New("cannot resolve attention_mask tensor name from model metadata")
	ErrCannotResolveOutput          = errors.New("cannot resolve output tensor name from model metadata")
	ErrTokenizerModelSectionMissing = errors.New("tokenizer.json: model section not found or empty vocab")
	ErrTokenizerVocabEmpty          = errors.New("tokenizer.json: vocab must not be empty")
	ErrTokenizerUnkTokenMissing     = errors.New("tokenizer.json: unk_token not found in vocab")
	ErrTokenizerCLSMissing          = errors.New(`tokenizer.json: special token "[CLS]" not found in vocab`)
	ErrTokenizerSEPMissing          = errors.New(`tokenizer.json: special token "[SEP]" not found in vocab`)
	ErrVocabEmpty                   = errors.New("vocab: must not be empty")
	ErrVocabUnkTokenMissing         = errors.New("vocab: unk_token not found in vocab")
	ErrVocabCLSMissing              = errors.New(`vocab: special token "[CLS]" not found`)
	ErrVocabSEPMissing              = errors.New(`vocab: special token "[SEP]" not found`)
	ErrTokenizerNil                 = errors.New("tokenizer must not be nil")
	ErrTokenizerPathRequired        = errors.New(
		"model profile requires a tokenizer asset, but no tokenizer_path was provided",
	)
	ErrTokenizerPathNotExist = errors.New(
		"model profile requires tokenizer, but file does not exist",
	)
	ErrTokenizerPathIsDirectory = errors.New(
		"model profile: tokenizer path is a directory, expected a file",
	)
	ErrTokenizerPathEmptyFile = errors.New(
		"model profile: tokenizer file is empty (0 bytes)",
	)
)
