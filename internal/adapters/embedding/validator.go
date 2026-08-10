package embedding

import (
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
		return ErrModelProfileEmptyAlt
	}

	modelPath := strings.TrimSpace(cfg.ModelPath)
	if modelPath == "" {
		return ErrModelPathEmpty
	}

	info, err := os.Stat(modelPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %q", ErrModelPathNotExist, modelPath)
		}

		return fmt.Errorf("model_path: cannot access %q: %w", modelPath, err)
	}

	if info.IsDir() {
		return fmt.Errorf("%w: %q", ErrModelPathIsDirectory, modelPath)
	}

	if info.Size() == 0 {
		return fmt.Errorf("%w: %q", ErrModelPathEmptyFile, modelPath)
	}

	profileCfg, err := ConfigFor(cfg.Profile, cfg.VectorDimension)
	if err != nil {
		return fmt.Errorf("model profile: %w", err)
	}

	if profileCfg.RequiresTokenizer {
		err := validateTokenizerAsset(cfg.Profile, cfg.TokenizerPath)
		if err != nil {
			return err
		}
	}

	if cfg.VectorDimension <= 0 {
		return fmt.Errorf("%w, got %d", ErrVectorDimensionInvalid, cfg.VectorDimension)
	}

	return nil
}

func validateTokenizerAsset(profile ModelProfile, tokenizerPath string) error {
	tokenizerPath = strings.TrimSpace(tokenizerPath)
	if tokenizerPath == "" {
		return fmt.Errorf("%w: %q", ErrTokenizerPathRequired, profile)
	}

	tokenizerInfo, err := os.Stat(tokenizerPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf(
				"%w: profile %q path %q",
				ErrTokenizerPathNotExist,
				profile,
				tokenizerPath,
			)
		}

		return fmt.Errorf(
			"model profile %q: cannot access tokenizer %q: %w",
			profile,
			tokenizerPath,
			err,
		)
	}

	if tokenizerInfo.IsDir() {
		return fmt.Errorf(
			"%w: profile %q path %q",
			ErrTokenizerPathIsDirectory,
			profile,
			tokenizerPath,
		)
	}

	if tokenizerInfo.Size() == 0 {
		return fmt.Errorf(
			"%w: profile %q path %q",
			ErrTokenizerPathEmptyFile,
			profile,
			tokenizerPath,
		)
	}

	return nil
}
