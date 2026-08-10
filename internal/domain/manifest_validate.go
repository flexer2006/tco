package domain

import (
	"fmt"
	"strings"
	"time"
)

func Validate(manifest Manifest) error {
	err := validateManifestHeader(manifest)
	if err != nil {
		return err
	}

	noteByID, canonicalByID, err := validateManifestNotes(manifest)
	if err != nil {
		return err
	}

	clusterByID, err := validateManifestClusters(manifest)
	if err != nil {
		return err
	}

	err = validateNoteClusterRefs(manifest, clusterByID, canonicalByID)
	if err != nil {
		return err
	}

	return validateClusterNoteMembership(manifest, noteByID)
}

func validateManifestHeader(manifest Manifest) error {
	if manifest.schemaVersion != SchemaVersion {
		return fmt.Errorf("%w %d", ErrUnsupportedSchemaVersion, manifest.schemaVersion)
	}

	if strings.TrimSpace(manifest.modelID) == "" {
		return ErrModelIDEmpty
	}

	if strings.TrimSpace(manifest.modelHash) == "" {
		return ErrModelHashEmpty
	}

	if strings.TrimSpace(manifest.modelProfile) == "" {
		return ErrModelProfileEmpty
	}

	if !IsSupportedNormalizationRule(manifest.normalizationRule) {
		return fmt.Errorf("%w %q", ErrUnsupportedNormalizationRule, manifest.normalizationRule)
	}

	if manifest.vectorDimension <= 0 {
		return fmt.Errorf("%w, got %d", ErrVectorDimensionInvalid, manifest.vectorDimension)
	}

	err := validateUTC("last_run_utc", manifest.lastRunUTC)
	if err != nil {
		return err
	}

	return validateRunMetadata(manifest.runMetadata)
}

func validateManifestNotes(
	manifest Manifest,
) (map[string]NoteRecord, map[string]NoteRecord, error) {
	noteByID := make(map[string]NoteRecord, len(manifest.notes))
	canonicalByID := make(map[string]NoteRecord, len(manifest.notes))

	for i, record := range manifest.notes {
		err := validateNoteRecord(record, manifest.vectorDimension)
		if err != nil {
			return nil, nil, fmt.Errorf("notes[%d]: %w", i, err)
		}

		id := record.ID().String()
		if _, exists := noteByID[id]; exists {
			return nil, nil, fmt.Errorf("notes[%d]: %w %q", i, ErrDuplicateNoteID, id)
		}

		noteByID[id] = record
		if record.DuplicateOf() == "" {
			canonicalByID[id] = record
		}
	}

	return noteByID, canonicalByID, nil
}

func validateManifestClusters(manifest Manifest) (map[string]ClusterRecord, error) {
	clusterByID := make(map[string]ClusterRecord, len(manifest.clusters))
	clusterBySlug := make(map[string]string, len(manifest.clusters))

	for i, record := range manifest.clusters {
		err := validateClusterRecord(record, manifest.vectorDimension)
		if err != nil {
			return nil, fmt.Errorf("clusters[%d]: %w", i, err)
		}

		id := record.ID()
		if _, exists := clusterByID[id]; exists {
			return nil, fmt.Errorf("clusters[%d]: %w %q", i, ErrDuplicateClusterID, id)
		}

		clusterByID[id] = record

		slug := record.Slug()
		if otherID, exists := clusterBySlug[slug]; exists {
			return nil, fmt.Errorf(
				"clusters[%d]: %w %q (also used by %q)",
				i,
				ErrDuplicateClusterSlug,
				slug,
				otherID,
			)
		}

		clusterBySlug[slug] = id
	}

	return clusterByID, nil
}

func validateNoteClusterRefs(
	manifest Manifest,
	clusterByID map[string]ClusterRecord,
	canonicalByID map[string]NoteRecord,
) error {
	for i, record := range manifest.notes {
		_, exists := clusterByID[record.ClusterID()]
		if !exists {
			return fmt.Errorf(
				"notes[%d]: %w %q",
				i,
				ErrClusterIDNotFound,
				record.ClusterID(),
			)
		}

		if record.DuplicateOf() == "" {
			continue
		}

		canonical, ok := canonicalByID[record.DuplicateOf().String()]
		if !ok {
			return fmt.Errorf(
				"notes[%d]: %w %q",
				i,
				ErrDuplicateOfNotFound,
				record.DuplicateOf(),
			)
		}

		if canonical.ClusterID() != record.ClusterID() {
			return fmt.Errorf(
				"notes[%d]: %w %q",
				i,
				ErrDuplicateOfClusterMismatch,
				record.DuplicateOf(),
			)
		}
	}

	return nil
}

func validateClusterNoteMembership(manifest Manifest, noteByID map[string]NoteRecord) error {
	for i, clusterRecord := range manifest.clusters {
		seen := make(map[string]struct{}, len(clusterRecord.NoteIDs()))
		for j, noteID := range clusterRecord.NoteIDs() {
			key := noteID.String()

			noteRecord, exists := noteByID[key]
			if !exists {
				return fmt.Errorf(
					"clusters[%d].note_ids[%d]: %w %q",
					i,
					j,
					ErrClusterNoteIDNotFound,
					key,
				)
			}

			if noteRecord.ClusterID() != clusterRecord.ID() {
				return fmt.Errorf(
					"clusters[%d].note_ids[%d]: %w %q belongs to %q, not %q",
					i,
					j,
					ErrClusterNoteClusterMismatch,
					key,
					noteRecord.ClusterID(),
					clusterRecord.ID(),
				)
			}

			seen[key] = struct{}{}
		}

		for _, noteRecord := range manifest.notes {
			if noteRecord.ClusterID() != clusterRecord.ID() {
				continue
			}

			if _, exists := seen[noteRecord.ID().String()]; !exists {
				return fmt.Errorf("clusters[%d]: %w %q", i, ErrClusterMissingNoteID, noteRecord.ID())
			}
		}
	}

	return nil
}

func validateRunMetadata(metadata RunMetadata) error {
	_, err := NewRunMetadata(
		metadata.RunID(),
		metadata.RunMode(),
		metadata.BatchMode(),
		metadata.BatchSize(),
		metadata.Thresholds(),
		metadata.Counts(),
		metadata.Timestamps(),
	)

	return err
}

func validateNoteRecord(record NoteRecord, vectorDimension int) error {
	validated, err := NewNoteRecord(
		record.ID(),
		record.SourceChat(),
		record.SourceMsgID(),
		record.Title(),
		record.Body(),
		record.EmbeddingID(),
		record.Embedding(),
		record.ClusterID(),
		record.Tags(),
		record.CreatedAt(),
		record.UpdatedAt(),
		record.DuplicateOf(),
	)
	if err != nil {
		return err
	}

	if validated.Embedding().Dimension() != vectorDimension {
		return fmt.Errorf(
			"%w: %d != %d",
			ErrEmbeddingDimensionMismatch,
			validated.Embedding().Dimension(),
			vectorDimension,
		)
	}

	return nil
}

func validateClusterRecord(record ClusterRecord, vectorDimension int) error {
	validated, err := newCluster(
		record.ID(),
		record.Name(),
		record.Slug(),
		record.Centroid(),
		record.NoteIDs(),
		record.CreatedAt(),
		record.UpdatedAt(),
	)
	if err != nil {
		return err
	}

	if validated.centroidValue().Dimension() != vectorDimension {
		return fmt.Errorf(
			"%w: %d != %d",
			ErrCentroidDimensionMismatch,
			validated.centroidValue().Dimension(),
			vectorDimension,
		)
	}

	return nil
}

func validateSimilarity(field string, value float64) error {
	if value <= 0 || value > 1 {
		return fmt.Errorf("%s: %w, got %v", field, ErrSimilarityOutOfRange, value)
	}

	return nil
}

func validateUTC(field string, value time.Time) error {
	if value.IsZero() {
		return fmt.Errorf("%s %w", field, ErrTimeMustNotBeZero)
	}

	if value.Location() != time.UTC {
		return fmt.Errorf("%s %w", field, ErrTimeMustBeUTC)
	}

	return nil
}
