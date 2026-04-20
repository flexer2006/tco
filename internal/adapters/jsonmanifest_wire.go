package adapters

import (
	"github.com/flexer2006/tco/internal/domain"
	"time"
)

type (
	manifestWire struct {
		RunMetadata       runMetadataWire `json:"run_metadata"`
		LastRunUTC        time.Time       `json:"last_run_utc"`
		Notes             []noteWire      `json:"notes"`
		Clusters          []clusterWire   `json:"clusters"`
		ModelID           string          `json:"model_id"`
		ModelHash         string          `json:"model_hash"`
		NormalizationRule string          `json:"normalization_rule"`
		ModelProfile      string          `json:"model_profile"`
		SchemaVersion     int             `json:"schema_version"`
		VectorDimension   int             `json:"vector_dimension"`
	}
	noteWire struct {
		Tags        []string  `json:"tags"`
		Embedding   []float32 `json:"embedding"`
		CreatedAt   time.Time `json:"created_at"`
		UpdatedAt   time.Time `json:"updated_at"`
		ID          string    `json:"id"`
		SourceChat  string    `json:"source_chat"`
		Title       string    `json:"title"`
		Body        string    `json:"body"`
		Hash        string    `json:"hash"`
		EmbeddingID string    `json:"embedding_id"`
		ClusterID   string    `json:"cluster_id"`
		DuplicateOf string    `json:"duplicate_of"`
		SourceMsgID int       `json:"source_msg_id"`
	}
	clusterWire struct {
		Centroid  []float32 `json:"centroid"`
		NoteIDs   []string  `json:"note_ids"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		ID        string    `json:"id"`
		Name      string    `json:"name"`
		Slug      string    `json:"slug"`
	}
	runMetadataWire struct {
		Timestamps timestampsWire   `json:"timestamps"`
		Counts     countsWire       `json:"counts"`
		RunID      string           `json:"run_id"`
		RunMode    domain.ModeRun   `json:"run_mode"`
		BatchMode  domain.ModeBatch `json:"batch_mode"`
		Thresholds thresholdsWire   `json:"thresholds"`
		BatchSize  int              `json:"batch_size"`
	}
	thresholdsWire struct {
		DedupSimilarity   float64 `json:"dedup_similarity"`
		ClusterSimilarity float64 `json:"cluster_similarity"`
	}
	countsWire struct {
		Notes          int `json:"notes"`
		CanonicalNotes int `json:"canonical_notes"`
		DuplicateNotes int `json:"duplicate_notes"`
		Clusters       int `json:"clusters"`
	}
	timestampsWire struct {
		StartedAtUTC  time.Time `json:"started_at_utc"`
		FinishedAtUTC time.Time `json:"finished_at_utc"`
	}
)
