package bootstrap

import (
	"net/http"
	"github.com/flexer2006/tco/internal/adapters"
	"github.com/flexer2006/tco/internal/application"
	"github.com/flexer2006/tco/internal/config"
	"github.com/flexer2006/tco/internal/domain"
)

type deps struct {
	LoadConfig             func() (config.Config, error)
	NewPolicy              func(domain.ModeRun, domain.ModeBatch, int) (domain.Policy, error)
	NewLiveSource          func(int, string, string, ...adapters.LiveSourceOption) (*adapters.LiveSource, error)
	NewLiveAuthGateway     func(string, ...adapters.LiveAuthGatewayOption) (*adapters.LiveAuthGateway, error)
	NewOrchestrator        func(application.TelegramSource, application.EmbeddingEncoder, application.ManifestStore, application.VaultProjector, domain.Policy, ...application.OrchestratorOption) (*application.Orchestrator, error)
	NewControlPlaneService func(application.PipelineRunner, string) (*application.Service, error)
	NewOnboardingService   func(string, ...application.ServiceOption) (*application.Service2, error)
	NewControlPlaneServer  func(string, int, adapters.Service, adapters.OnboardingService) (*http.Server, error)
	BuildEmbeddingEncoder  func(config.Config) (application.EmbeddingEncoder, error)
}

func defaultDeps() deps {
	return deps{
		LoadConfig:             config.Load,
		NewPolicy:              domain.NewPolicy,
		NewLiveSource:          adapters.NewLiveSource,
		NewLiveAuthGateway:     adapters.NewLiveAuthGateway,
		NewOrchestrator:        application.NewOrchestrator,
		NewControlPlaneService: application.NewService,
		NewOnboardingService:   application.NewService2,
		NewControlPlaneServer:  adapters.NewServer,
		BuildEmbeddingEncoder:  newProductionEncoder,
	}
}

func (deps deps) withDefaults() deps {
	defaults := defaultDeps()
	if deps.LoadConfig == nil {
		deps.LoadConfig = defaults.LoadConfig
	}
	if deps.NewPolicy == nil {
		deps.NewPolicy = defaults.NewPolicy
	}
	if deps.NewLiveSource == nil {
		deps.NewLiveSource = defaults.NewLiveSource
	}
	if deps.NewLiveAuthGateway == nil {
		deps.NewLiveAuthGateway = defaults.NewLiveAuthGateway
	}
	if deps.NewOrchestrator == nil {
		deps.NewOrchestrator = defaults.NewOrchestrator
	}
	if deps.NewControlPlaneService == nil {
		deps.NewControlPlaneService = defaults.NewControlPlaneService
	}
	if deps.NewOnboardingService == nil {
		deps.NewOnboardingService = defaults.NewOnboardingService
	}
	if deps.NewControlPlaneServer == nil {
		deps.NewControlPlaneServer = defaults.NewControlPlaneServer
	}
	if deps.BuildEmbeddingEncoder == nil {
		deps.BuildEmbeddingEncoder = defaults.BuildEmbeddingEncoder
	}
	return deps
}
