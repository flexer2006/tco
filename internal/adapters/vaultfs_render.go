package adapters

import (
	"encoding/json"
	"strings"
	"github.com/flexer2006/tco/internal/domain"
)

type embeddingSidecar struct {
	NoteID            string    `json:"note_id"`
	ModelID           string    `json:"model_id"`
	ModelHash         string    `json:"model_hash"`
	VectorDimension   int       `json:"vector_dimension"`
	NormalizationRule string    `json:"normalization_rule"`
	Values            []float32 `json:"values"`
}

func renderNoteRecordMarkdown(record domain.NoteRecord) (string, error) {
	return domain.RenderNoteMarkdown(
		record.ID(),
		record.SourceChat(),
		record.SourceMsgID(),
		record.Title(),
		record.Body(),
		record.EmbeddingID(),
		record.ClusterID(),
		record.Tags(),
		record.CreatedAt(),
		record.UpdatedAt(),
		record.DuplicateOf(),
	)
}

func renderClusterIndex(clusterRecord domain.ClusterRecord, notesByID map[string]domain.NoteRecord) (string, error) {
	var b strings.Builder
	b.Grow(256)
	b.WriteString("# ")
	b.WriteString(clusterRecord.Name())
	b.WriteString("\n\n")
	b.WriteString("## Canonical notes\n\n")
	for _, noteID := range clusterRecord.NoteIDs() {
		record := notesByID[noteID.String()]
		if record.DuplicateOf() != "" {
			continue
		}
		b.WriteString("- [")
		b.WriteString(record.Title())
		b.WriteString("](")
		b.WriteString(record.ID().String())
		b.WriteString(".md)\n")
		duplicates := duplicateNotesFor(record.ID(), clusterRecord.NoteIDs(), notesByID)
		if len(duplicates) == 0 {
			continue
		}
		b.WriteString("  - duplicates:\n")
		for _, duplicate := range duplicates {
			b.WriteString("    - `")
			b.WriteString(duplicate.ID().String())
			b.WriteString("`\n")
		}
	}
	return b.String(), nil
}

func duplicateNotesFor(canonicalID domain.NoteID, orderedIDs []domain.NoteID, notesByID map[string]domain.NoteRecord) []domain.NoteRecord {
	duplicates := make([]domain.NoteRecord, 0)
	for _, noteID := range orderedIDs {
		record := notesByID[noteID.String()]
		if record.DuplicateOf() == canonicalID {
			duplicates = append(duplicates, record)
		}
	}
	return duplicates
}

func marshalSidecarDeterministic(noteID, modelID, modelHash string, vectorDimension int, normalizationRule string, values []float32) []byte {
	sidecar := embeddingSidecar{
		NoteID:            noteID,
		ModelID:           modelID,
		ModelHash:         modelHash,
		VectorDimension:   vectorDimension,
		NormalizationRule: normalizationRule,
		Values:            values,
	}
	raw, _ := json.MarshalIndent(sidecar, "", "  ")
	return append(raw, '\n')
}
