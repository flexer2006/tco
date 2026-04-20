package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"github.com/flexer2006/tco/internal/adapters"
	"github.com/flexer2006/tco/internal/application"
	"github.com/flexer2006/tco/internal/config"
)

func buildEmbeddingEncoderWithDeps(cfg config.Config, deps deps) (application.EmbeddingEncoder, error) {
	deps = deps.withDefaults()
	if cfg.RuntimeProfile != runtimeProfileReal {
		return nil, fmt.Errorf("unsupported RUNTIME_PROFILE: %q", cfg.RuntimeProfile)
	}
	return deps.BuildEmbeddingEncoder(cfg)
}

func newProductionEncoder(cfg config.Config) (application.EmbeddingEncoder, error) {
	profile, err := adapters.Parse(cfg.EmbedModelProfile)
	if err != nil {
		return nil, fmt.Errorf("initialize production ONNX encoder: %w", err)
	}
	profileCfg, err := adapters.ConfigFor(profile, cfg.EmbedVectorDimension)
	if err != nil {
		return nil, fmt.Errorf("initialize production ONNX encoder: %w", err)
	}
	tokenizerPath := resolveTokenizerPath(cfg.EmbedModelPath)
	if err := adapters.Validate(adapters.Config{
		Profile:         profile,
		ModelPath:       cfg.EmbedModelPath,
		TokenizerPath:   tokenizerPath,
		VectorDimension: profileCfg.VectorDimension,
	}); err != nil {
		return nil, fmt.Errorf("initialize production ONNX encoder: model validation failed: %w", err)
	}
	switch profile {
	case adapters.ProfileStringInputDirect:
		return newStringDirectEncoder(cfg, profileCfg)
	case adapters.ProfileBertTokenizedMeanPooling:
		return newBertTokenizedEncoder(cfg, profileCfg, tokenizerPath)
	default:
		return nil, fmt.Errorf("initialize production ONNX encoder: unsupported profile %q", profile)
	}
}

func newStringDirectEncoder(cfg config.Config, profileCfg adapters.ProfileConfig) (application.EmbeddingEncoder, error) {
	runtime := defaultNativeRuntimeFactory(cfg)
	encoder, err := adapters.NewEncoder(
		cfg.EmbedModelID,
		cfg.EmbedModelPath,
		profileCfg.VectorDimension,
		runtime,
		adapters.WithModelProfile(string(adapters.ProfileStringInputDirect)),
	)
	if err != nil {
		return nil, fmt.Errorf("initialize production ONNX encoder: %w", err)
	}
	return encoder, nil
}

func newBertTokenizedEncoder(cfg config.Config, profileCfg adapters.ProfileConfig, tokenizerPath string) (application.EmbeddingEncoder, error) {
	tokenizer, err := loadTokenizer(tokenizerPath)
	if err != nil {
		return nil, fmt.Errorf("initialize production ONNX encoder: load tokenizer for bert_tokenized_mean_pooling profile: %w", err)
	}
	profiledRuntime := adapters.NewProfiledRuntime(
		adapters.ProfileBertTokenizedMeanPooling,
		adapters.WithProfiledSharedLibraryPath(cfg.ONNXRuntimeSharedLibrary),
		adapters.WithProfiledTokenizer(tokenizerPath, tokenizer),
	)
	encoder, err := adapters.NewEncoder(
		cfg.EmbedModelID,
		cfg.EmbedModelPath,
		profileCfg.VectorDimension,
		profiledRuntime,
		adapters.WithModelProfile(string(adapters.ProfileBertTokenizedMeanPooling)),
	)
	if err != nil {
		return nil, fmt.Errorf("initialize production ONNX encoder: %w", err)
	}
	return encoder, nil
}

func resolveTokenizerPath(modelPath string) string {
	dir := filepath.Dir(modelPath)
	base := strings.TrimSuffix(filepath.Base(modelPath), filepath.Ext(modelPath))
	candidates := []string{
		filepath.Join(dir, "tokenizer.json"),
		filepath.Join(dir, base+"_tokenizer.json"),
		filepath.Join(dir, "vocab.txt"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return filepath.Join(dir, "tokenizer.json")
}

func loadTokenizer(path string) (*adapters.Tokenizer, error) {
	if strings.HasSuffix(path, "tokenizer.json") {
		return adapters.NewTokenizer(path)
	}
	return adapters.NewTokenizerFromVocab(path)
}

func defaultNativeRuntimeFactory(cfg config.Config) adapters.Runtime {
	return adapters.NewNativeRuntime(
		adapters.WithNativeSharedLibraryPath(cfg.ONNXRuntimeSharedLibrary),
		adapters.WithNativeInputOutputNames(cfg.ONNXInputName, cfg.ONNXOutputName),
	)
}
