package adapters

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"github.com/flexer2006/tco/internal/application"
	"github.com/flexer2006/tco/internal/domain"
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

func NewProjector(vaultRoot string) (application.VaultProjector, error) {
	if strings.TrimSpace(vaultRoot) == "" {
		return nil, errors.New("vault root must not be empty")
	}
	projector := &projector{
		root:             filepath.Clean(vaultRoot),
		atomic:           newAtomicWriter(),
		embeddingPath:    embeddingSidecarPath,
		clusterIndexPath: clusterIndexPath,
		noteMarkdownPath: noteMarkdownPath,
		marshalSidecar: func(noteID, modelID, modelHash string, vectorDimension int, normalizationRule string, values []float32) ([]byte, error) {
			return marshalSidecarDeterministic(noteID, modelID, modelHash, vectorDimension, normalizationRule, values), nil
		},
		renderClusterIndex: renderClusterIndex,
		renderNoteMarkdown: renderNoteRecordMarkdown,
		pruneManagedFiles:  pruneManagedFiles,
	}
	return projector, nil
}

//nolint:gocyclo
func (p *projector) Project(value domain.Manifest) (application.ProjectionStats, error) {
	if p == nil {
		return application.ProjectionStats{}, errors.New("vaultfs projector must not be nil")
	}
	if err := domain.Validate(value); err != nil {
		return application.ProjectionStats{}, err
	}
	notesByID := make(map[string]domain.NoteRecord, len(value.Notes()))
	clusterByID := make(map[string]domain.ClusterRecord, len(value.Clusters()))
	for _, record := range value.Clusters() {
		clusterByID[record.ID()] = record
	}
	for _, record := range value.Notes() {
		notesByID[record.ID().String()] = record
	}
	stats := application.ProjectionStats{}
	desired := make(map[string]struct{})
	for _, record := range value.Notes() {
		if record.DuplicateOf() != "" {
			continue
		}
		sidecarPath, err := p.embeddingPath(p.root, record.ID().String())
		if err != nil {
			return application.ProjectionStats{}, err
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
		raw, err := p.marshalSidecar(sidecar.NoteID, sidecar.ModelID, sidecar.ModelHash, sidecar.VectorDimension, sidecar.NormalizationRule, sidecar.Values)
		if err != nil {
			return application.ProjectionStats{}, err
		}
		changed, err := p.atomic.write(sidecarPath, raw)
		if err != nil {
			return application.ProjectionStats{}, fmt.Errorf("write embedding sidecar %q: %w", sidecarPath, err)
		}
		if changed {
			stats.Written++
		} else {
			stats.Skipped++
		}
	}
	for _, clusterRecord := range value.Clusters() {
		indexPath, err := p.clusterIndexPath(p.root, clusterRecord.Slug())
		if err != nil {
			return application.ProjectionStats{}, err
		}
		desired[indexPath] = struct{}{}
		rendered, err := p.renderClusterIndex(clusterRecord, notesByID)
		if err != nil {
			return application.ProjectionStats{}, err
		}
		changed, err := p.atomic.write(indexPath, []byte(rendered))
		if err != nil {
			return application.ProjectionStats{}, fmt.Errorf("write cluster index %q: %w", indexPath, err)
		}
		if changed {
			stats.Written++
		} else {
			stats.Skipped++
		}
	}
	for _, record := range value.Notes() {
		if record.DuplicateOf() != "" {
			continue
		}
		clusterRecord := clusterByID[record.ClusterID()]
		notePath, err := p.noteMarkdownPath(p.root, clusterRecord.Slug(), record.ID().String())
		if err != nil {
			return application.ProjectionStats{}, err
		}
		desired[notePath] = struct{}{}
		noteMarkdown, err := p.renderNoteMarkdown(record)
		if err != nil {
			return application.ProjectionStats{}, err
		}
		changed, err := p.atomic.write(notePath, []byte(noteMarkdown))
		if err != nil {
			return application.ProjectionStats{}, fmt.Errorf("write note %q: %w", notePath, err)
		}
		if changed {
			stats.Written++
		} else {
			stats.Skipped++
		}
	}
	pruned, err := p.pruneManagedFiles(p.root, desired)
	if err != nil {
		return application.ProjectionStats{}, err
	}
	stats.Pruned = pruned
	return stats, nil
}
