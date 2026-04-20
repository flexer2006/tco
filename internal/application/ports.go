package application

import (
	"context"
	"github.com/flexer2006/tco/internal/domain"
)

type (
	ManifestStore interface {
		Load() (domain.Manifest, error)
		Save(domain.Manifest) (bool, error)
	}
	VaultProjector interface {
		Project(domain.Manifest) (ProjectionStats, error)
	}
	TelegramSource interface {
		FetchMessages(ctx context.Context, sourceChat string) ([]domain.RawCanonicalMessage, error)
	}
	EmbeddingEncoder interface {
		Encode(ctx context.Context, texts []string) ([]domain.Vector, error)
		Metadata() EmbeddingMetadata
	}
	ProjectionStats struct {
		Written, Skipped, Pruned int
	}
	EmbeddingMetadata struct {
		ModelID, ModelHash, ModelProfile, NormalizationRule string
		VectorDimension                                     int
	}
)
