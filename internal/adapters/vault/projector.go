package vault

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/flexer2006/tco/internal/domain"
	"github.com/flexer2006/tco/internal/ports"
)

type (
	projector struct {
		atomic                          atomicWriter
		root                            string
		clusterIndexPath, embeddingPath func(string, string) (string, error)
		noteMarkdownPath                func(string, string, string) (string, error)
		marshalSidecar                  func(string, string, string, int, string, []float32) ([]byte, error)
		renderClusterIndex              func(domain.ClusterRecord, map[string]domain.NoteRecord) (string, error)
		renderNoteMarkdown              func(domain.NoteRecord) (string, error)
		pruneManagedFiles               func(string, map[string]struct{}) (int, error)
	}
)

func NewProjector(vaultRoot string) (ports.VaultProjector, error) {
	if strings.TrimSpace(vaultRoot) == "" {
		return nil, ErrVaultRootEmpty
	}

	projector := new(projector{
		root:             filepath.Clean(vaultRoot),
		atomic:           newAtomicWriter(),
		embeddingPath:    embeddingSidecarPath,
		clusterIndexPath: clusterIndexPath,
		noteMarkdownPath: noteMarkdownPath,
		marshalSidecar: func(
			noteID, modelID, modelHash string,
			vectorDimension int,
			normalizationRule string,
			values []float32,
		) ([]byte, error) {
			return marshalSidecarDeterministic(
				noteID,
				modelID,
				modelHash,
				vectorDimension,
				normalizationRule,
				values,
			), nil
		},
		renderClusterIndex: renderClusterIndex,
		renderNoteMarkdown: renderNoteRecordMarkdown,
		pruneManagedFiles:  pruneManagedFiles,
	})

	return projector, nil
}

func (p *projector) Project(value domain.Manifest) (ports.ProjectionStats, error) {
	if p == nil {
		return ports.ProjectionStats{}, ErrProjectorNil
	}

	err := domain.Validate(value)
	if err != nil {
		return ports.ProjectionStats{}, err
	}

	notes := value.Notes()
	clusters := value.Clusters()
	notesByID := make(map[string]domain.NoteRecord, len(notes))
	clusterByID := make(map[string]domain.ClusterRecord, len(clusters))

	for _, record := range clusters {
		clusterByID[record.ID()] = record
	}

	for _, record := range notes {
		notesByID[record.ID().String()] = record
	}

	stats := ports.ProjectionStats{}
	desired := make(map[string]struct{})

	err = p.projectSidecars(value, notes, desired, &stats)
	if err != nil {
		return ports.ProjectionStats{}, err
	}

	err = p.projectClusterIndexes(clusters, notesByID, desired, &stats)
	if err != nil {
		return ports.ProjectionStats{}, err
	}

	err = p.projectNotes(notes, clusterByID, desired, &stats)
	if err != nil {
		return ports.ProjectionStats{}, err
	}

	pruned, err := p.pruneManagedFiles(p.root, desired)
	if err != nil {
		return ports.ProjectionStats{}, err
	}

	stats.Pruned = pruned

	return stats, nil
}

func (p *projector) projectSidecars(
	value domain.Manifest,
	notes []domain.NoteRecord,
	desired map[string]struct{},
	stats *ports.ProjectionStats,
) error {
	for _, record := range notes {
		if record.DuplicateOf() != "" {
			continue
		}

		sidecarPath, err := p.embeddingPath(p.root, record.ID().String())
		if err != nil {
			return err
		}

		desired[sidecarPath] = struct{}{}
		sidecar := embeddingSidecar{
			NoteID:            record.ID().String(),
			ModelID:           value.ModelID(),
			ModelHash:         value.ModelHash(),
			VectorDimension:   value.VectorDimension(),
			NormalizationRule: value.NormalizationRule(),
			Values:            record.Embedding().Values(),
		}

		raw, err := p.marshalSidecar(
			sidecar.NoteID,
			sidecar.ModelID,
			sidecar.ModelHash,
			sidecar.VectorDimension,
			sidecar.NormalizationRule,
			sidecar.Values,
		)
		if err != nil {
			return err
		}

		changed, err := p.atomic.write(sidecarPath, raw)
		if err != nil {
			return fmt.Errorf("write embedding sidecar %q: %w", sidecarPath, err)
		}

		if changed {
			stats.Written++
		} else {
			stats.Skipped++
		}
	}

	return nil
}

func (p *projector) projectClusterIndexes(
	clusters []domain.ClusterRecord,
	notesByID map[string]domain.NoteRecord,
	desired map[string]struct{},
	stats *ports.ProjectionStats,
) error {
	for _, clusterRecord := range clusters {
		indexPath, err := p.clusterIndexPath(p.root, clusterRecord.Slug())
		if err != nil {
			return err
		}

		desired[indexPath] = struct{}{}

		rendered, err := p.renderClusterIndex(clusterRecord, notesByID)
		if err != nil {
			return err
		}

		changed, err := p.atomic.write(indexPath, []byte(rendered))
		if err != nil {
			return fmt.Errorf("write cluster index %q: %w", indexPath, err)
		}

		if changed {
			stats.Written++
		} else {
			stats.Skipped++
		}
	}

	return nil
}

func (p *projector) projectNotes(
	notes []domain.NoteRecord,
	clusterByID map[string]domain.ClusterRecord,
	desired map[string]struct{},
	stats *ports.ProjectionStats,
) error {
	for _, record := range notes {
		if record.DuplicateOf() != "" {
			continue
		}

		clusterRecord := clusterByID[record.ClusterID()]

		notePath, err := p.noteMarkdownPath(p.root, clusterRecord.Slug(), record.ID().String())
		if err != nil {
			return err
		}

		desired[notePath] = struct{}{}

		noteMarkdown, err := p.renderNoteMarkdown(record)
		if err != nil {
			return err
		}

		changed, err := p.atomic.write(notePath, []byte(noteMarkdown))
		if err != nil {
			return fmt.Errorf("write note %q: %w", notePath, err)
		}

		if changed {
			stats.Written++
		} else {
			stats.Skipped++
		}
	}

	return nil
}
