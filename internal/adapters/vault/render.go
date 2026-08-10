package vault

import (
	"encoding/json"
	"strings"

	"github.com/flexer2006/tco/internal/domain"
)

type embeddingSidecar struct {
	Values            []float32 `json:"values"`
	NoteID            string    `json:"note_id"`
	ModelID           string    `json:"model_id"`
	ModelHash         string    `json:"model_hash"`
	NormalizationRule string    `json:"normalization_rule"`
	VectorDimension   int       `json:"vector_dimension"`
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

func renderClusterIndex(
	clusterRecord domain.ClusterRecord,
	notesByID map[string]domain.NoteRecord,
) (string, error) {
	const clusterIndexGrow = 256

	var builder strings.Builder
	builder.Grow(clusterIndexGrow)
	builder.WriteString("# ")
	builder.WriteString(clusterRecord.Name())
	builder.WriteString("\n\n")
	builder.WriteString("## Canonical notes\n\n")

	for _, noteID := range clusterRecord.NoteIDs() {
		record := notesByID[noteID.String()]
		if record.DuplicateOf() != "" {
			continue
		}

		builder.WriteString("- [")
		builder.WriteString(record.Title())
		builder.WriteString("](")
		builder.WriteString(record.ID().String())
		builder.WriteString(".md)\n")

		duplicates := duplicateNotesFor(record.ID(), clusterRecord.NoteIDs(), notesByID)
		if len(duplicates) == 0 {
			continue
		}

		builder.WriteString("  - duplicates:\n")

		for _, duplicate := range duplicates {
			builder.WriteString("    - `")
			builder.WriteString(duplicate.ID().String())
			builder.WriteString("`\n")
		}
	}

	return builder.String(), nil
}

func duplicateNotesFor(
	canonicalID domain.NoteID,
	orderedIDs []domain.NoteID,
	notesByID map[string]domain.NoteRecord,
) []domain.NoteRecord {
	duplicates := make([]domain.NoteRecord, 0)

	for _, noteID := range orderedIDs {
		record := notesByID[noteID.String()]
		if record.DuplicateOf() == canonicalID {
			duplicates = append(duplicates, record)
		}
	}

	return duplicates
}

func marshalSidecarDeterministic(
	noteID, modelID, modelHash string,
	vectorDimension int,
	normalizationRule string,
	values []float32,
) []byte {
	sidecar := embeddingSidecar{
		NoteID:            noteID,
		ModelID:           modelID,
		ModelHash:         modelHash,
		VectorDimension:   vectorDimension,
		NormalizationRule: normalizationRule,
		Values:            values,
	}

	raw, err := json.MarshalIndent(sidecar, "", "  ")
	if err != nil {
		return []byte("{}\n")
	}

	return append(raw, '\n')
}
