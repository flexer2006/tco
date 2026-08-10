package embedding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/flexer2006/tco/internal/domain"
	"github.com/flexer2006/tco/internal/ports"
)

type (
	encoderOptions struct {
		modelHash, modelProfile, normalizationRule string
		readFile                                   func(string) ([]byte, error)
	}
	EncoderOption func(*encoderOptions)
	encoder       struct {
		metadata  ports.EmbeddingMetadata
		runtime   Runtime
		modelPath string
	}
	Runtime interface {
		Encode(ctx context.Context, modelPath string, texts []string) ([][]float32, error)
	}
)

func WithModelProfile(modelProfile string) EncoderOption {
	return func(options *encoderOptions) {
		options.modelProfile = modelProfile
	}
}

func NewEncoder(
	modelID, modelPath string,
	vectorDimension int,
	runtime Runtime,
	options ...EncoderOption,
) (ports.EmbeddingEncoder, error) {
	if strings.TrimSpace(modelID) == "" {
		return nil, ErrModelIDEmpty
	}

	if strings.TrimSpace(modelPath) == "" {
		return nil, ErrModelPathEmpty
	}

	if vectorDimension <= 0 {
		return nil, fmt.Errorf("%w, got %d", ErrVectorDimensionInvalid, vectorDimension)
	}

	if runtime == nil {
		return nil, ErrRuntimeNil
	}

	config := encoderOptions{
		readFile:          os.ReadFile,
		normalizationRule: domain.NormalizationRuleL2Unit,
	}

	for _, option := range options {
		if option != nil {
			option(&config)
		}
	}

	modelHash := strings.TrimSpace(config.modelHash)
	if modelHash == "" {
		raw, err := config.readFile(modelPath)
		if err != nil {
			return nil, fmt.Errorf("read model file %q: %w", modelPath, err)
		}

		sum := sha256.Sum256(raw)
		modelHash = hex.EncodeToString(sum[:])
	}

	metadata := ports.EmbeddingMetadata{
		ModelID:           modelID,
		ModelHash:         modelHash,
		ModelProfile:      config.modelProfile,
		VectorDimension:   vectorDimension,
		NormalizationRule: config.normalizationRule,
	}

	err := validateMetadata(metadata)
	if err != nil {
		return nil, err
	}

	return new(encoder{
		metadata:  metadata,
		runtime:   runtime,
		modelPath: modelPath,
	}), nil
}

func (e *encoder) Encode(ctx context.Context, texts []string) ([]domain.Vector, error) {
	if e == nil {
		return nil, ErrONNXEncoderNil
	}

	if ctx == nil {
		return nil, ErrContextNil
	}

	err := ctx.Err()
	if err != nil {
		return nil, fmt.Errorf("encode embeddings: %w", err)
	}

	if len(texts) == 0 {
		return []domain.Vector{}, nil
	}

	rawVectors, err := e.runtime.Encode(ctx, e.modelPath, texts)
	if err != nil {
		return nil, fmt.Errorf("encode embeddings: %w", err)
	}

	if len(rawVectors) != len(texts) {
		return nil, fmt.Errorf(
			"%w: %d vectors for %d texts",
			ErrRuntimeVectorCount,
			len(rawVectors),
			len(texts),
		)
	}

	vectors := make([]domain.Vector, 0, len(rawVectors))
	for i, raw := range rawVectors {
		if len(raw) != e.metadata.VectorDimension {
			return nil, fmt.Errorf(
				"%w: vector[%d] has dimension %d (expected %d)",
				ErrRuntimeVectorDimension,
				i,
				len(raw),
				e.metadata.VectorDimension,
			)
		}

		vector, err := domain.NewVectorAlreadyNormalized(raw)
		if err != nil {
			return nil, fmt.Errorf("runtime returned invalid vector[%d]: %w", i, err)
		}

		vectors = append(vectors, vector)
	}

	return vectors, nil
}

func (e *encoder) Metadata() ports.EmbeddingMetadata {
	if e == nil {
		return ports.EmbeddingMetadata{}
	}

	return e.metadata
}

func validateMetadata(metadata ports.EmbeddingMetadata) error {
	if strings.TrimSpace(metadata.ModelID) == "" {
		return ErrModelIDEmpty
	}

	if strings.TrimSpace(metadata.ModelHash) == "" {
		return ErrModelHashEmpty
	}

	if metadata.VectorDimension <= 0 {
		return fmt.Errorf(
			"%w, got %d",
			ErrVectorDimensionInvalid,
			metadata.VectorDimension,
		)
	}

	if !domain.IsSupportedNormalizationRule(metadata.NormalizationRule) {
		return fmt.Errorf("%w %q", ErrUnsupportedNormRule, metadata.NormalizationRule)
	}

	return nil
}
