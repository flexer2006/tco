package bootstrap

import (
	"github.com/flexer2006/tco/internal/adapters/vault"
	"github.com/flexer2006/tco/internal/application/pipeline"
	"github.com/flexer2006/tco/internal/config"
	"github.com/flexer2006/tco/internal/domain"
)

func (c container) orchestrator(cfg config.Config) (*pipeline.Orchestrator, error) {
	deps := c.withDefaults()

	policy, err := deps.NewPolicy(
		domain.ModeRun(cfg.RunMode),
		domain.ModeBatch(cfg.BatchMode),
		cfg.BatchSize,
	)
	if err != nil {
		return nil, err
	}

	source, err := deps.telegramSource(cfg)
	if err != nil {
		return nil, err
	}

	manifestStore, err := vault.NewStore(cfg.ManifestPath)
	if err != nil {
		return nil, err
	}

	vaultProjector, err := vault.NewProjector(cfg.VaultRoot)
	if err != nil {
		return nil, err
	}

	dirtyMarker, err := vault.NewProjectionDirtyMarker(cfg.VaultRoot)
	if err != nil {
		return nil, err
	}

	encoder, err := deps.embeddingEncoder(cfg)
	if err != nil {
		return nil, err
	}

	return deps.NewOrchestrator(
		source,
		encoder,
		manifestStore,
		vaultProjector,
		policy,
		pipeline.WithThresholds(cfg.DedupSimilarityThreshold, cfg.ClusterSimilarityThreshold),
		pipeline.WithProjectionDirtyMarker(dirtyMarker),
		pipeline.WithHistoryMaxMessages(cfg.HistoryMaxMessages),
	)
}
