package adapters

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/flexer2006/tco/internal/application"
	"github.com/flexer2006/tco/internal/domain"
)

type (
	encoderOptions struct {
		readFile                                   func(string) ([]byte, error)
		modelHash, modelProfile, normalizationRule string
	}
	EncoderOption func(*encoderOptions)
	encoder       struct {
		metadata  application.EmbeddingMetadata
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

func NewEncoder(modelID, modelPath string, vectorDimension int, runtime Runtime, options ...EncoderOption) (application.EmbeddingEncoder, error) {
	if strings.TrimSpace(modelID) == "" {
		return nil, errors.New("model_id: must not be empty")
	}
	if strings.TrimSpace(modelPath) == "" {
		return nil, errors.New("model_path: must not be empty")
	}
	if vectorDimension <= 0 {
		return nil, fmt.Errorf("vector_dimension: must be greater than 0, got %d", vectorDimension)
	}
	if runtime == nil {
		return nil, errors.New("runtime must not be nil")
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
		modelHash = fmt.Sprintf("%x", sum[:])
	}
	metadata := application.EmbeddingMetadata{
		ModelID:           modelID,
		ModelHash:         modelHash,
		ModelProfile:      config.modelProfile,
		VectorDimension:   vectorDimension,
		NormalizationRule: config.normalizationRule,
	}
	if err := validateMetadata(metadata); err != nil {
		return nil, err
	}
	return &encoder{
		metadata:  metadata,
		runtime:   runtime,
		modelPath: modelPath,
	}, nil
}

func (e *encoder) Encode(ctx context.Context, texts []string) ([]domain.Vector, error) {
	if e == nil {
		return nil, errors.New("onnx encoder must not be nil")
	}
	if ctx == nil {
		return nil, errors.New("context must not be nil")
	}
	if err := ctx.Err(); err != nil {
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
		return nil, fmt.Errorf("runtime returned %d vectors for %d texts", len(rawVectors), len(texts))
	}
	vectors := make([]domain.Vector, 0, len(rawVectors))
	for i, raw := range rawVectors {
		if len(raw) != e.metadata.VectorDimension {
			return nil, fmt.Errorf("runtime returned vector[%d] with dimension %d (expected %d)", i, len(raw), e.metadata.VectorDimension)
		}
		vector, err := domain.NewVector(raw)
		if err != nil {
			return nil, fmt.Errorf("runtime returned invalid vector[%d]: %w", i, err)
		}
		vectors = append(vectors, vector)
	}
	return vectors, nil
}

func (e *encoder) Metadata() application.EmbeddingMetadata {
	if e == nil {
		return application.EmbeddingMetadata{}
	}
	return e.metadata
}

func validateMetadata(metadata application.EmbeddingMetadata) error {
	if strings.TrimSpace(metadata.ModelID) == "" {
		return fmt.Errorf("model_id: must not be empty")
	}
	if strings.TrimSpace(metadata.ModelHash) == "" {
		return fmt.Errorf("model_hash: must not be empty")
	}
	if metadata.VectorDimension <= 0 {
		return fmt.Errorf("vector_dimension: must be greater than 0, got %d", metadata.VectorDimension)
	}
	if !domain.IsSupportedNormalizationRule(metadata.NormalizationRule) {
		return fmt.Errorf("normalization_rule: unsupported value %q", metadata.NormalizationRule)
	}
	return nil
}
