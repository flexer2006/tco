package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

//nolint:gocyclo
func Validate(m Manifest) error {
	if m.schemaVersion != SchemaVersion {
		return fmt.Errorf("schema_version: unsupported value %d", m.schemaVersion)
	}
	if strings.TrimSpace(m.modelID) == "" {
		return errors.New("model_id must not be empty")
	}
	if strings.TrimSpace(m.modelHash) == "" {
		return errors.New("model_hash must not be empty")
	}
	if strings.TrimSpace(m.modelProfile) == "" {
		return errors.New("model_profile must not be empty")
	}
	if !IsSupportedNormalizationRule(m.normalizationRule) {
		return fmt.Errorf("normalization_rule: unsupported value %q", m.normalizationRule)
	}
	if m.vectorDimension <= 0 {
		return fmt.Errorf("vector_dimension: must be greater than 0, got %d", m.vectorDimension)
	}
	if err := validateUTC("last_run_utc", m.lastRunUTC); err != nil {
		return err
	}
	if err := validateRunMetadata(m.runMetadata); err != nil {
		return err
	}
	noteByID := make(map[string]NoteRecord, len(m.notes))
	canonicalByID := make(map[string]NoteRecord, len(m.notes))
	for i, record := range m.notes {
		if err := validateNoteRecord(record, m.vectorDimension); err != nil {
			return fmt.Errorf("notes[%d]: %w", i, err)
		}
		id := record.ID().String()
		if _, exists := noteByID[id]; exists {
			return fmt.Errorf("notes[%d]: duplicate note id %q", i, id)
		}
		noteByID[id] = record
		if record.DuplicateOf() == "" {
			canonicalByID[id] = record
		}
	}
	clusterByID := make(map[string]ClusterRecord, len(m.clusters))
	for i, record := range m.clusters {
		if err := validateClusterRecord(record, m.vectorDimension); err != nil {
			return fmt.Errorf("clusters[%d]: %w", i, err)
		}
		id := record.ID()
		if _, exists := clusterByID[id]; exists {
			return fmt.Errorf("clusters[%d]: duplicate cluster id %q", i, id)
		}
		clusterByID[id] = record
	}
	for i, record := range m.notes {
		_, exists := clusterByID[record.ClusterID()]
		if !exists {
			return fmt.Errorf("notes[%d]: cluster_id %q does not reference an existing cluster", i, record.ClusterID())
		}
		if record.DuplicateOf() != "" {
			canonical, ok := canonicalByID[record.DuplicateOf().String()]
			if !ok {
				return fmt.Errorf("notes[%d]: duplicate_of %q does not reference an existing canonical note", i, record.DuplicateOf())
			}
			if canonical.ClusterID() != record.ClusterID() {
				return fmt.Errorf("notes[%d]: duplicate_of %q must belong to the same cluster", i, record.DuplicateOf())
			}
		}
	}
	for i, clusterRecord := range m.clusters {
		seen := make(map[string]struct{}, len(clusterRecord.NoteIDs()))
		for j, noteID := range clusterRecord.NoteIDs() {
			key := noteID.String()
			noteRecord, exists := noteByID[key]
			if !exists {
				return fmt.Errorf("clusters[%d].note_ids[%d]: note id %q does not reference an existing note", i, j, key)
			}
			if noteRecord.ClusterID() != clusterRecord.ID() {
				return fmt.Errorf("clusters[%d].note_ids[%d]: note id %q belongs to cluster %q, not %q", i, j, key, noteRecord.ClusterID(), clusterRecord.ID())
			}
			seen[key] = struct{}{}
		}
		for _, noteRecord := range m.notes {
			if noteRecord.ClusterID() != clusterRecord.ID() {
				continue
			}
			if _, exists := seen[noteRecord.ID().String()]; !exists {
				return fmt.Errorf("clusters[%d]: missing note id %q", i, noteRecord.ID())
			}
		}
	}
	return nil
}

func validateRunMetadata(metadata RunMetadata) error {
	_, err := NewRunMetadata(metadata.RunID(), metadata.RunMode(), metadata.BatchMode(), metadata.BatchSize(), metadata.Thresholds(), metadata.Counts(), metadata.Timestamps())
	return err
}

func validateNoteRecord(record NoteRecord, vectorDimension int) error {
	validated, err := NewNoteRecord(record.ID(), record.SourceChat(), record.SourceMsgID(), record.Title(), record.Body(), record.EmbeddingID(), record.Embedding(), record.ClusterID(), record.Tags(), record.CreatedAt(), record.UpdatedAt(), record.DuplicateOf())
	if err != nil {
		return err
	}
	if validated.Embedding().Dimension() != vectorDimension {
		return fmt.Errorf("embedding dimension %d does not match vector_dimension %d", validated.Embedding().Dimension(), vectorDimension)
	}
	return nil
}

func validateClusterRecord(record ClusterRecord, vectorDimension int) error {
	validated, err := newCluster(record.ID(), record.Name(), record.Slug(), record.Centroid(), record.NoteIDs(), record.CreatedAt(), record.UpdatedAt())
	if err != nil {
		return err
	}
	if validated.centroidValue().Dimension() != vectorDimension {
		return fmt.Errorf("centroid dimension %d does not match vector_dimension %d", validated.centroidValue().Dimension(), vectorDimension)
	}
	return nil
}

func validateSimilarity(field string, value float64) error {
	if value <= 0 || value > 1 {
		return fmt.Errorf("%s: must be within (0, 1], got %v", field, value)
	}
	return nil
}

func validateUTC(field string, value time.Time) error {
	if value.IsZero() {
		return fmt.Errorf("%s must not be zero", field)
	}
	if value.Location() != time.UTC {
		return fmt.Errorf("%s must be UTC", field)
	}
	return nil
}
