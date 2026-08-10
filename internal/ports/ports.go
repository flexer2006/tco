package ports

import (
	"context"

	"github.com/flexer2006/tco/internal/domain"
)

type (
	ProjectionStats struct {
		Written, Skipped, Pruned int
	}
	EmbeddingMetadata struct {
		ModelID, ModelHash, ModelProfile, NormalizationRule string
		VectorDimension                                     int
	}
	MessageFetchRequest struct {
		SourceChat            string
		MaxMessages           int
		MinExclusiveMessageID int
	}
	ManifestStore interface {
		Load() (domain.Manifest, error)
		Save(manifest domain.Manifest) (bool, error)
	}
	VaultProjector interface {
		Project(manifest domain.Manifest) (ProjectionStats, error)
	}
	ProjectionDirtyMarker interface {
		IsDirty() (bool, error)
		MarkDirty() error
		ClearDirty() error
	}
	TelegramSource interface {
		FetchMessages(
			ctx context.Context,
			req MessageFetchRequest,
		) ([]domain.RawCanonicalMessage, error)
	}
	EmbeddingEncoder interface {
		Encode(ctx context.Context, texts []string) ([]domain.Vector, error)
		Metadata() EmbeddingMetadata
	}
)
