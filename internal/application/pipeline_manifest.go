package application

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"github.com/flexer2006/tco/internal/domain"
	"time"
)

func (o *Orchestrator) buildManifest(sourceChat string, messages []domain.RawCanonicalMessage, result dedupClusterResult, meta EmbeddingMetadata) (domain.Manifest, error) {
	if err := validateEncoderMetadata(meta); err != nil {
		return domain.Manifest{}, err
	}
	rawByID := indexRawMessagesByID(messages)
	noteBodies := indexSemanticMasterBodies(result.Notes)
	noteRecords, clusterMembers, canonicalCount, err := buildManifestNoteRecords(result.Notes, rawByID, noteBodies, meta)
	if err != nil {
		return domain.Manifest{}, err
	}
	clusterRecords, err := buildManifestClusterRecords(result.Clusters, clusterMembers, messages)
	if err != nil {
		return domain.Manifest{}, err
	}
	runMetadata, err := o.buildManifestRunMetadata(sourceChat, messages, meta, len(noteRecords), canonicalCount, len(clusterRecords))
	if err != nil {
		return domain.Manifest{}, err
	}
	latestDate := latestDate(messages)
	return domain.NewManifestWithModelProfile(
		domain.SchemaVersion,
		meta.ModelID,
		meta.ModelHash,
		effectiveModelProfile(meta),
		meta.VectorDimension,
		meta.NormalizationRule,
		latestDate,
		noteRecords,
		clusterRecords,
		runMetadata,
	)
}

func indexRawMessagesByID(messages []domain.RawCanonicalMessage) map[string]domain.RawCanonicalMessage {
	rawByID := make(map[string]domain.RawCanonicalMessage, len(messages))
	for _, raw := range messages {
		rawByID[raw.SourceChat()+":"+fmt.Sprint(raw.SourceMsgID())] = raw
	}
	return rawByID
}

func indexSemanticMasterBodies(notes []dedupClusterNote) map[string]string {
	noteBodies := make(map[string]string, len(notes))
	for _, note := range notes {
		if note.IsSemanticMaster {
			noteBodies[note.NoteID.String()] = note.Body
		}
	}
	return noteBodies
}

func buildManifestNoteRecords(
	notes []dedupClusterNote,
	rawByID map[string]domain.RawCanonicalMessage,
	noteBodies map[string]string,
	meta EmbeddingMetadata,
) ([]domain.NoteRecord, map[string][]domain.NoteID, int, error) {
	noteRecords := make([]domain.NoteRecord, 0, len(notes))
	clusterMembers := make(map[string][]domain.NoteID)
	canonicalCount := 0

	for _, note := range notes {
		raw := rawByID[note.SourceChat+":"+fmt.Sprint(note.SourceMsgID)]
		createdAt := raw.DateUTC().UTC()
		updatedAt := createdAt
		if editedAt := raw.EditedAtUTC(); editedAt != nil && editedAt.After(updatedAt) {
			updatedAt = editedAt.UTC()
		}
		body := note.Body
		if !note.IsSemanticMaster {
			body = noteBodies[note.SemanticMasterID.String()]
		}
		noteRecord, err := domain.NewNoteRecord(
			note.NoteID,
			note.SourceChat,
			note.SourceMsgID,
			note.Title,
			body,
			meta.ModelID,
			note.Embedding,
			note.ClusterID,
			[]string{},
			createdAt,
			updatedAt,
			note.DuplicateOf,
		)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("build Manifest note record for note %q: %w", note.NoteID, err)
		}
		noteRecords = append(noteRecords, noteRecord)
		clusterMembers[note.ClusterID] = append(clusterMembers[note.ClusterID], note.NoteID)
		if note.DuplicateOf == "" {
			canonicalCount++
		}
	}

	return noteRecords, clusterMembers, canonicalCount, nil
}

func buildManifestClusterRecords(
	clusters []dedupCluster,
	clusterMembers map[string][]domain.NoteID,
	messages []domain.RawCanonicalMessage,
) ([]domain.ClusterRecord, error) {
	clusterRecords := make([]domain.ClusterRecord, 0, len(clusters))
	latestDate := latestDate(messages)
	for _, dedupCluster := range clusters {
		noteIDs := clusterMembers[dedupCluster.ClusterID]
		slices.SortFunc(noteIDs, func(left, right domain.NoteID) int { return strings.Compare(left.String(), right.String()) })
		clusterRecord, err := domain.NewClusterRecord(
			dedupCluster.ClusterID,
			dedupCluster.Name,
			dedupCluster.Slug,
			dedupCluster.Centroid,
			noteIDs,
			latestDate,
			latestDate,
		)
		if err != nil {
			return nil, fmt.Errorf("build Manifest cluster record for cluster %q: %w", dedupCluster.ClusterID, err)
		}
		clusterRecords = append(clusterRecords, clusterRecord)
	}
	return clusterRecords, nil
}

func (o *Orchestrator) buildManifestRunMetadata(
	sourceChat string,
	messages []domain.RawCanonicalMessage,
	meta EmbeddingMetadata,
	noteCount, canonicalCount, clusterCount int,
) (domain.RunMetadata, error) {
	thresholds, err := domain.NewThresholds(o.DedupThreshold, o.ClusterThreshold)
	if err != nil {
		return domain.RunMetadata{}, fmt.Errorf("build Manifest thresholds: %w", err)
	}
	counts, err := domain.NewCounts(noteCount, canonicalCount, noteCount-canonicalCount, clusterCount)
	if err != nil {
		return domain.RunMetadata{}, fmt.Errorf("build Manifest counts: %w", err)
	}
	timestamps, err := domain.NewTimestamps(earliestDate(messages), latestDate(messages))
	if err != nil {
		return domain.RunMetadata{}, fmt.Errorf("build Manifest timestamps: %w", err)
	}
	runMetadata, err := domain.NewRunMetadata(
		deterministicRunID(sourceChat, messages, meta, o.Policy),
		o.Policy.RunMode(),
		o.Policy.BatchMode(),
		o.Policy.BatchSize(),
		thresholds,
		counts,
		timestamps,
	)
	if err != nil {
		return domain.RunMetadata{}, fmt.Errorf("build Manifest run metadata: %w", err)
	}
	return runMetadata, nil
}

func ensureManifestMetadataCompatibility(previous domain.Manifest, current EmbeddingMetadata, runMode domain.ModeRun) error {
	if err := validateEncoderMetadata(current); err != nil {
		return err
	}
	if previous.SchemaVersion() == 0 {
		return nil
	}
	if runMode != domain.Incremental {
		return nil
	}
	var mismatches []string
	mismatches = appendStringMismatch(mismatches, "model_id", previous.ModelID(), current.ModelID)
	mismatches = appendStringMismatch(mismatches, "model_hash", previous.ModelHash(), current.ModelHash)
	mismatches = appendStringMismatch(mismatches, "model_profile", previous.ModelProfile(), effectiveModelProfile(current))
	mismatches = appendIntMismatch(mismatches, "vector_dimension", previous.VectorDimension(), current.VectorDimension)
	mismatches = appendStringMismatch(mismatches, "normalization_rule", previous.NormalizationRule(), current.NormalizationRule)
	if len(mismatches) == 0 {
		return nil
	}
	return fmt.Errorf("incremental run metadata mismatch with existing Manifest (%s); rerun with RUN_MODE=full_rebuild to regenerate Manifest and vault", strings.Join(mismatches, "; "))
}

func appendStringMismatch(mismatches []string, field, existing, current string) []string {
	if existing == current {
		return mismatches
	}
	return append(mismatches, fmt.Sprintf("%s existing=%q current=%q", field, existing, current))
}

func appendIntMismatch(mismatches []string, field string, existing, current int) []string {
	if existing == current {
		return mismatches
	}
	return append(mismatches, fmt.Sprintf("%s existing=%d current=%d", field, existing, current))
}

func validateEncoderMetadata(meta EmbeddingMetadata) error {
	if strings.TrimSpace(meta.ModelID) == "" {
		return errors.New("encoder metadata model_id must not be empty")
	}
	if strings.TrimSpace(meta.ModelHash) == "" {
		return errors.New("encoder metadata model_hash must not be empty")
	}
	if meta.VectorDimension <= 0 {
		return fmt.Errorf("encoder metadata vector_dimension must be greater than 0, got %d", meta.VectorDimension)
	}
	if !domain.IsSupportedNormalizationRule(meta.NormalizationRule) {
		return fmt.Errorf("encoder metadata normalization_rule: unsupported value %q", meta.NormalizationRule)
	}
	return nil
}

func effectiveModelProfile(meta EmbeddingMetadata) string {
	modelProfile := strings.TrimSpace(meta.ModelProfile)
	if modelProfile == "" {
		return "bert_tokenized_mean_pooling"
	}
	return modelProfile
}

func deterministicRunID(sourceChat string, messages []domain.RawCanonicalMessage, meta EmbeddingMetadata, policy domain.Policy) string {
	h := sha256.New()
	_, _ = h.Write([]byte(sourceChat))
	_, _ = h.Write([]byte(meta.ModelID))
	_, _ = h.Write([]byte(meta.ModelHash))
	_, _ = h.Write([]byte(effectiveModelProfile(meta)))
	_, _ = fmt.Fprintf(h, "%s/%s/%d", policy.RunMode(), policy.BatchMode(), policy.BatchSize())
	for _, raw := range messages {
		_, _ = h.Write([]byte(raw.SourceChat()))
		_, _ = fmt.Fprintf(h, "%d", raw.SourceMsgID())
		_, _ = h.Write([]byte(raw.Text()))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func earliestDate(messages []domain.RawCanonicalMessage) time.Time {
	if len(messages) == 0 {
		return time.Unix(0, 0).UTC()
	}
	earliest := messages[0].DateUTC().UTC()
	for _, raw := range messages[1:] {
		if raw.DateUTC().Before(earliest) {
			earliest = raw.DateUTC().UTC()
		}
	}
	return earliest
}

func latestDate(messages []domain.RawCanonicalMessage) time.Time {
	if len(messages) == 0 {
		return time.Unix(0, 0).UTC()
	}
	latest := messages[0].DateUTC().UTC()
	for _, raw := range messages[1:] {
		if raw.DateUTC().After(latest) {
			latest = raw.DateUTC().UTC()
		}
	}
	return latest
}
