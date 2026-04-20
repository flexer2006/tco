package adapters

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Profile         ModelProfile
	ModelPath       string
	TokenizerPath   string
	VectorDimension int
}

func Validate(cfg Config) error {
	if cfg.Profile == "" {
		return errors.New("model profile: must not be empty")
	}
	modelPath := strings.TrimSpace(cfg.ModelPath)
	if modelPath == "" {
		return errors.New("model_path: must not be empty")
	}
	info, err := os.Stat(modelPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("model_path: file %q does not exist", modelPath)
		}
		return fmt.Errorf("model_path: cannot access %q: %w", modelPath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("model_path: %q is a directory, expected an ONNX model file", modelPath)
	}
	if info.Size() == 0 {
		return fmt.Errorf("model_path: %q is empty (0 bytes)", modelPath)
	}
	profileCfg, err := ConfigFor(cfg.Profile, cfg.VectorDimension)
	if err != nil {
		return fmt.Errorf("model profile: %w", err)
	}
	if profileCfg.RequiresTokenizer {
		tokenizerPath := strings.TrimSpace(cfg.TokenizerPath)
		if tokenizerPath == "" {
			return fmt.Errorf("model profile %q requires a tokenizer asset, but no tokenizer_path was provided", cfg.Profile)
		}
		tokenizerInfo, err := os.Stat(tokenizerPath)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("model profile %q requires tokenizer at %q, but file does not exist", cfg.Profile, tokenizerPath)
			}
			return fmt.Errorf("model profile %q: cannot access tokenizer %q: %w", cfg.Profile, tokenizerPath, err)
		}
		if tokenizerInfo.IsDir() {
			return fmt.Errorf("model profile %q: tokenizer path %q is a directory, expected a file", cfg.Profile, tokenizerPath)
		}
		if tokenizerInfo.Size() == 0 {
			return fmt.Errorf("model profile %q: tokenizer file %q is empty (0 bytes)", cfg.Profile, tokenizerPath)
		}
	}
	if cfg.VectorDimension <= 0 {
		return fmt.Errorf("vector_dimension: must be greater than 0, got %d", cfg.VectorDimension)
	}
	return nil
}
