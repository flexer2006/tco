package bootstrap

import (
	"github.com/flexer2006/tco/internal/adapters"
	"github.com/flexer2006/tco/internal/application"
	"github.com/flexer2006/tco/internal/config"
	"github.com/flexer2006/tco/internal/domain"
)

func buildOrchestratorWithDeps(cfg config.Config, deps deps) (*application.Orchestrator, error) {
	deps = deps.withDefaults()
	policy, err := deps.NewPolicy(domain.ModeRun(cfg.RunMode), domain.ModeBatch(cfg.BatchMode), cfg.BatchSize)
	if err != nil {
		return nil, err
	}
	source, err := buildTelegramSourceWithDeps(cfg, deps)
	if err != nil {
		return nil, err
	}
	manifestStore, err := adapters.NewStore(cfg.ManifestPath)
	if err != nil {
		return nil, err
	}
	vaultProjector, err := adapters.NewProjector(cfg.VaultRoot)
	if err != nil {
		return nil, err
	}
	encoder, err := buildEmbeddingEncoderWithDeps(cfg, deps)
	if err != nil {
		return nil, err
	}
	orchestrator, err := deps.NewOrchestrator(
		source,
		encoder,
		manifestStore,
		vaultProjector,
		policy,
		application.WithThresholds(cfg.DedupSimilarityThreshold, cfg.ClusterSimilarityThreshold),
	)
	if err != nil {
		return nil, err
	}
	return orchestrator, nil
}
