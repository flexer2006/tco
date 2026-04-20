package adapters

import (
	"errors"
	"fmt"
	"strings"
	"github.com/flexer2006/tco/internal/domain"
)

func manifestToWire(value domain.Manifest) manifestWire {
	notes, clusters := value.Notes(), value.Clusters()
	wireNotes := make([]noteWire, 0, len(notes))
	for _, record := range notes {
		wireNotes = append(wireNotes, noteToWire(record))
	}
	wireClusters := make([]clusterWire, 0, len(clusters))
	for _, record := range clusters {
		wireClusters = append(wireClusters, clusterToWire(record))
	}
	runMetadata := runMetadataToWire(value.RunMetadata())
	return manifestWire{
		SchemaVersion:     value.SchemaVersion(),
		ModelID:           value.ModelID(),
		ModelHash:         value.ModelHash(),
		ModelProfile:      value.ModelProfile(),
		VectorDimension:   value.VectorDimension(),
		NormalizationRule: value.NormalizationRule(),
		LastRunUTC:        value.LastRunUTC().UTC(),
		Notes:             wireNotes,
		Clusters:          wireClusters,
		RunMetadata:       runMetadata,
	}
}

func wireToManifest(value manifestWire) (domain.Manifest, error) {
	if value.SchemaVersion != domain.SchemaVersion {
		return domain.Manifest{}, fmt.Errorf("schema_version: unsupported value %d", value.SchemaVersion)
	}
	notes := make([]domain.NoteRecord, 0, len(value.Notes))
	for i, record := range value.Notes {
		noteRecord, err := noteFromWire(record)
		if err != nil {
			return domain.Manifest{}, fmt.Errorf("notes[%d]: %w", i, err)
		}
		notes = append(notes, noteRecord)
	}
	clusters := make([]domain.ClusterRecord, 0, len(value.Clusters))
	for i, record := range value.Clusters {
		clusterRecord, err := clusterFromWire(record)
		if err != nil {
			return domain.Manifest{}, fmt.Errorf("clusters[%d]: %w", i, err)
		}
		clusters = append(clusters, clusterRecord)
	}
	runMetadata, err := runMetadataFromWire(value.RunMetadata)
	if err != nil {
		return domain.Manifest{}, err
	}
	normalizationRule := strings.TrimSpace(value.NormalizationRule)
	if normalizationRule == "" {
		normalizationRule = domain.DefaultNormalizationRule
	}
	modelProfile := strings.TrimSpace(value.ModelProfile)
	if modelProfile == "" {
		modelProfile = "bert_tokenized_mean_pooling"
	}
	return domain.NewManifestWithModelProfile(
		value.SchemaVersion,
		value.ModelID,
		value.ModelHash,
		modelProfile,
		value.VectorDimension,
		normalizationRule,
		value.LastRunUTC.UTC(),
		notes,
		clusters,
		runMetadata,
	)
}

func noteToWire(record domain.NoteRecord) noteWire {
	embeddingValues := record.Embedding().Values()
	tags := record.Tags()
	if len(tags) == 0 {
		tags = []string{}
	}
	return noteWire{
		ID:          record.ID().String(),
		SourceChat:  record.SourceChat(),
		SourceMsgID: record.SourceMsgID(),
		Title:       record.Title(),
		Body:        record.Body(),
		Hash:        record.Hash(),
		EmbeddingID: record.EmbeddingID(),
		Embedding:   embeddingValues,
		ClusterID:   record.ClusterID(),
		Tags:        tags,
		CreatedAt:   record.CreatedAt().UTC(),
		UpdatedAt:   record.UpdatedAt().UTC(),
		DuplicateOf: record.DuplicateOf().String(),
	}
}

func clusterToWire(record domain.ClusterRecord) clusterWire {
	return clusterWire{
		ID:        record.ID(),
		Name:      record.Name(),
		Slug:      record.Slug(),
		Centroid:  record.Centroid().Values(),
		NoteIDs:   stringNoteIDs(record.NoteIDs()),
		CreatedAt: record.CreatedAt().UTC(),
		UpdatedAt: record.UpdatedAt().UTC(),
	}
}

func runMetadataToWire(record domain.RunMetadata) runMetadataWire {
	thresholds := record.Thresholds()
	counts := record.Counts()
	timestamps := record.Timestamps()
	return runMetadataWire{
		RunID:     record.RunID(),
		RunMode:   record.RunMode(),
		BatchMode: record.BatchMode(),
		BatchSize: record.BatchSize(),
		Thresholds: thresholdsWire{
			DedupSimilarity:   thresholds.DedupSimilarity(),
			ClusterSimilarity: thresholds.ClusterSimilarity(),
		},
		Counts: countsWire{
			Notes:          counts.Notes(),
			CanonicalNotes: counts.CanonicalNotes(),
			DuplicateNotes: counts.DuplicateNotes(),
			Clusters:       counts.Clusters(),
		},
		Timestamps: timestampsWire{
			StartedAtUTC:  timestamps.StartedAtUTC().UTC(),
			FinishedAtUTC: timestamps.FinishedAtUTC().UTC(),
		},
	}
}

func noteFromWire(record noteWire) (domain.NoteRecord, error) {
	embeddingValue, err := domain.NewVector(record.Embedding)
	if err != nil {
		return domain.NoteRecord{}, err
	}
	noteRecord, err := domain.NewNoteRecord(domain.NoteID(record.ID), record.SourceChat, record.SourceMsgID, record.Title, record.Body, record.EmbeddingID, embeddingValue, record.ClusterID, record.Tags, record.CreatedAt.UTC(), record.UpdatedAt.UTC(), domain.NoteID(record.DuplicateOf))
	if err != nil {
		return domain.NoteRecord{}, err
	}
	if noteRecord.Hash() != record.Hash {
		return domain.NoteRecord{}, errors.New("hash does not match rendered body")
	}
	return noteRecord, nil
}

func clusterFromWire(record clusterWire) (domain.ClusterRecord, error) {
	noteIDs := make([]domain.NoteID, 0, len(record.NoteIDs))
	for _, noteID := range record.NoteIDs {
		noteIDs = append(noteIDs, domain.NoteID(noteID))
	}
	centroid, err := domain.NewVector(record.Centroid)
	if err != nil {
		return domain.ClusterRecord{}, err
	}
	return domain.NewClusterRecord(record.ID, record.Name, record.Slug, centroid, noteIDs, record.CreatedAt.UTC(), record.UpdatedAt.UTC())
}

func runMetadataFromWire(record runMetadataWire) (domain.RunMetadata, error) {
	thresholds, err := domain.NewThresholds(record.Thresholds.DedupSimilarity, record.Thresholds.ClusterSimilarity)
	if err != nil {
		return domain.RunMetadata{}, err
	}
	counts, err := domain.NewCounts(record.Counts.Notes, record.Counts.CanonicalNotes, record.Counts.DuplicateNotes, record.Counts.Clusters)
	if err != nil {
		return domain.RunMetadata{}, err
	}
	timestamps, err := domain.NewTimestamps(record.Timestamps.StartedAtUTC.UTC(), record.Timestamps.FinishedAtUTC.UTC())
	if err != nil {
		return domain.RunMetadata{}, err
	}
	return domain.NewRunMetadata(record.RunID, record.RunMode, record.BatchMode, record.BatchSize, thresholds, counts, timestamps)
}

func stringNoteIDs(noteIDs []domain.NoteID) []string {
	result := make([]string, 0, len(noteIDs))
	for _, noteID := range noteIDs {
		result = append(result, noteID.String())
	}
	return result
}
