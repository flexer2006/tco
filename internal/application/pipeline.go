package application

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"github.com/flexer2006/tco/internal/domain"
)

type (
	Orchestrator struct {
		Policy                           domain.Policy
		Source                           TelegramSource
		Encoder                          EmbeddingEncoder
		ManifestStore                    ManifestStore
		VaultProjector                   VaultProjector
		DedupThreshold, ClusterThreshold float64
	}
	RunOutcome struct {
		Manifest                           domain.Manifest
		ProjectionStats                    ProjectionStats
		ManifestChanged, ProjectionChanged bool
	}
	OrchestratorOption func(*Orchestrator)
)

func WithThresholds(dedupThreshold, clusterThreshold float64) OrchestratorOption {
	return func(orchestrator *Orchestrator) {
		orchestrator.DedupThreshold = dedupThreshold
		orchestrator.ClusterThreshold = clusterThreshold
	}
}

func NewOrchestrator(source TelegramSource, encoder EmbeddingEncoder, manifestStore ManifestStore, vaultProjector VaultProjector, policy domain.Policy, options ...OrchestratorOption) (*Orchestrator, error) {
	if source == nil {
		return nil, errors.New("source must not be nil")
	}
	if encoder == nil {
		return nil, errors.New("encoder must not be nil")
	}
	if manifestStore == nil {
		return nil, errors.New("manifest store must not be nil")
	}
	if vaultProjector == nil {
		return nil, errors.New("vault projector must not be nil")
	}
	if _, err := domain.NewPolicy(policy.RunMode(), policy.BatchMode(), policy.BatchSize()); err != nil {
		return nil, err
	}
	orchestrator := &Orchestrator{Source: source, Encoder: encoder, ManifestStore: manifestStore, VaultProjector: vaultProjector, Policy: policy, DedupThreshold: 0.95, ClusterThreshold: 0.80}
	for _, option := range options {
		if option == nil {
			continue
		}
		option(orchestrator)
	}
	return orchestrator, nil
}

//nolint:gocyclo
func (o *Orchestrator) Run(ctx context.Context, sourceChat string) (RunOutcome, error) {
	if o == nil {
		return RunOutcome{}, errors.New("orchestrator must not be nil")
	}
	if ctx == nil {
		return RunOutcome{}, errors.New("context must not be nil")
	}
	if strings.TrimSpace(sourceChat) == "" {
		return RunOutcome{}, errors.New("source chat must not be empty")
	}
	if err := ctx.Err(); err != nil {
		return RunOutcome{}, err
	}
	previousManifest, loadErr := o.ManifestStore.Load()
	if loadErr != nil {
		if !errors.Is(loadErr, os.ErrNotExist) {
			return RunOutcome{}, fmt.Errorf("load existing Manifest: %w", loadErr)
		}
		previousManifest = domain.Manifest{}
	}
	meta := o.Encoder.Metadata()
	if err := ensureManifestMetadataCompatibility(previousManifest, meta, o.Policy.RunMode()); err != nil {
		return RunOutcome{}, err
	}
	messages, err := o.Source.FetchMessages(ctx, sourceChat)
	if err != nil {
		return RunOutcome{}, err
	}
	inputs, err := o.encode(ctx, messages)
	if err != nil {
		return RunOutcome{}, err
	}
	dedupResult, err := runDedupClustering(inputs, o.DedupThreshold, o.ClusterThreshold)
	if err != nil {
		return RunOutcome{}, err
	}
	manifestValue, err := o.buildManifest(sourceChat, messages, dedupResult, meta)
	if err != nil {
		return RunOutcome{}, err
	}
	changed := !reflect.DeepEqual(previousManifest, manifestValue)
	if changed {
		changed, err = o.ManifestStore.Save(manifestValue)
		if err != nil {
			return RunOutcome{}, err
		}
	}
	projectionStats := ProjectionStats{}
	if changed {
		projectionStats, err = o.VaultProjector.Project(manifestValue)
		if err != nil {
			return RunOutcome{}, err
		}
	}
	return RunOutcome{Manifest: manifestValue, ManifestChanged: changed, ProjectionChanged: changed, ProjectionStats: projectionStats}, nil
}
