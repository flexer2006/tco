package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/flexer2006/tco/internal/adapters/embedding"
	"github.com/flexer2006/tco/internal/config"
	"github.com/flexer2006/tco/internal/ports"
)

func (c container) embeddingEncoder(cfg config.Config) (ports.EmbeddingEncoder, error) {
	deps := c.withDefaults()

	if cfg.RuntimeProfile != runtimeProfileReal {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedRuntimeProfile, cfg.RuntimeProfile)
	}

	return deps.BuildEmbeddingEncoder(cfg)
}

func newProductionEncoder(cfg config.Config) (ports.EmbeddingEncoder, error) {
	profile, err := embedding.Parse(cfg.EmbedModelProfile)
	if err != nil {
		return nil, fmt.Errorf("initialize production ONNX encoder: %w", err)
	}

	profileCfg, err := embedding.ConfigFor(profile, cfg.EmbedVectorDimension)
	if err != nil {
		return nil, fmt.Errorf("initialize production ONNX encoder: %w", err)
	}

	tokenizerPath := resolveTokenizerPath(cfg.EmbedModelPath)
	err = embedding.Validate(embedding.Config{
		Profile:         profile,
		ModelPath:       cfg.EmbedModelPath,
		TokenizerPath:   tokenizerPath,
		VectorDimension: profileCfg.VectorDimension,
	})
	if err != nil {
		return nil, fmt.Errorf(
			"initialize production ONNX encoder: model validation failed: %w",
			err,
		)
	}

	switch profile {
	case embedding.ProfileStringInputDirect:
		return newStringDirectEncoder(cfg, profileCfg)
	case embedding.ProfileBertTokenizedMeanPooling:
		return newBertTokenizedEncoder(cfg, profileCfg, tokenizerPath)
	default:
		return nil, fmt.Errorf("%w %q", ErrUnsupportedEncoderProfile, profile)
	}
}

func newStringDirectEncoder(
	cfg config.Config,
	profileCfg embedding.ProfileConfig,
) (ports.EmbeddingEncoder, error) {
	runtime := embedding.NewProfiledRuntime(
		embedding.ProfileStringInputDirect,
		embedding.WithProfiledSharedLibraryPath(cfg.ONNXRuntimeSharedLibrary),
	)

	encoder, err := embedding.NewEncoder(
		cfg.EmbedModelID,
		cfg.EmbedModelPath,
		profileCfg.VectorDimension,
		runtime,
		embedding.WithModelProfile(string(embedding.ProfileStringInputDirect)),
	)
	if err != nil {
		return nil, fmt.Errorf("initialize production ONNX encoder: %w", err)
	}

	return encoder, nil
}

func newBertTokenizedEncoder(
	cfg config.Config,
	profileCfg embedding.ProfileConfig,
	tokenizerPath string,
) (ports.EmbeddingEncoder, error) {
	tokenizer, err := loadTokenizer(tokenizerPath)
	if err != nil {
		return nil, fmt.Errorf(
			"initialize production ONNX encoder: load tokenizer for bert_tokenized_mean_pooling profile: %w",
			err,
		)
	}

	profiledRuntime := embedding.NewProfiledRuntime(
		embedding.ProfileBertTokenizedMeanPooling,
		embedding.WithProfiledSharedLibraryPath(cfg.ONNXRuntimeSharedLibrary),
		embedding.WithProfiledTokenizer(tokenizerPath, tokenizer),
	)

	encoder, err := embedding.NewEncoder(
		cfg.EmbedModelID,
		cfg.EmbedModelPath,
		profileCfg.VectorDimension,
		profiledRuntime,
		embedding.WithModelProfile(string(embedding.ProfileBertTokenizedMeanPooling)),
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
		_, err := os.Stat(candidate)
		if err == nil {
			return candidate
		}
	}

	return filepath.Join(dir, "tokenizer.json")
}

func loadTokenizer(path string) (*embedding.Tokenizer, error) {
	if strings.HasSuffix(path, "tokenizer.json") {
		return embedding.NewTokenizer(path)
	}

	return embedding.NewTokenizerFromVocab(path)
}
