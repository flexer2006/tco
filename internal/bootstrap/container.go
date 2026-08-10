package bootstrap

import (
	"net/http"

	"github.com/flexer2006/tco/internal/adapters/httpcontrol"
	"github.com/flexer2006/tco/internal/adapters/telegram"
	"github.com/flexer2006/tco/internal/application/control"
	"github.com/flexer2006/tco/internal/application/onboarding"
	"github.com/flexer2006/tco/internal/application/pipeline"
	"github.com/flexer2006/tco/internal/config"
	"github.com/flexer2006/tco/internal/domain"
	"github.com/flexer2006/tco/internal/ports"
)

type container struct {
	LoadConfig         func() (config.Config, error)
	NewPolicy          func(domain.ModeRun, domain.ModeBatch, int) (domain.Policy, error)
	NewLiveSource      func(int, string, string, ...telegram.LiveSourceOption) (*telegram.LiveSource, error)
	NewLiveAuthGateway func(string, ...telegram.LiveAuthGatewayOption) (*telegram.LiveAuthGateway, error)
	NewOrchestrator    func(
		ports.TelegramSource,
		ports.EmbeddingEncoder,
		ports.ManifestStore,
		ports.VaultProjector,
		domain.Policy,
		...pipeline.OrchestratorOption,
	) (*pipeline.Orchestrator, error)
	NewControlPlaneService func(control.PipelineRunner, string) (*control.Service, error)
	NewOnboardingService   func(string, ...onboarding.ServiceOption) (*onboarding.Onboarding, error)
	NewControlPlaneServer  func(
		string,
		int,
		httpcontrol.Service,
		httpcontrol.OnboardingService,
		string,
	) (*http.Server, error)
	BuildEmbeddingEncoder func(config.Config) (ports.EmbeddingEncoder, error)
}

func newContainer() container {
	return container{
		LoadConfig:             config.Load,
		NewPolicy:              domain.NewPolicy,
		NewLiveSource:          telegram.NewLiveSource,
		NewLiveAuthGateway:     telegram.NewLiveAuthGateway,
		NewOrchestrator:        pipeline.NewOrchestrator,
		NewControlPlaneService: control.NewService,
		NewOnboardingService:   onboarding.NewOnboarding,
		NewControlPlaneServer:  newControlPlaneServer,
		BuildEmbeddingEncoder:  newProductionEncoder,
	}
}

func newControlPlaneServer(
	bind string,
	port int,
	service httpcontrol.Service,
	onboardingSvc httpcontrol.OnboardingService,
	controlToken string,
) (*http.Server, error) {
	return httpcontrol.NewServer(
		bind,
		port,
		service,
		onboardingSvc,
		httpcontrol.ServerOptions{ControlPlaneToken: controlToken},
	)
}

func (c container) withDefaults() container {
	defaults := newContainer()
	if c.LoadConfig == nil {
		c.LoadConfig = defaults.LoadConfig
	}
	if c.NewPolicy == nil {
		c.NewPolicy = defaults.NewPolicy
	}
	if c.NewLiveSource == nil {
		c.NewLiveSource = defaults.NewLiveSource
	}
	if c.NewLiveAuthGateway == nil {
		c.NewLiveAuthGateway = defaults.NewLiveAuthGateway
	}
	if c.NewOrchestrator == nil {
		c.NewOrchestrator = defaults.NewOrchestrator
	}
	if c.NewControlPlaneService == nil {
		c.NewControlPlaneService = defaults.NewControlPlaneService
	}
	if c.NewOnboardingService == nil {
		c.NewOnboardingService = defaults.NewOnboardingService
	}
	if c.NewControlPlaneServer == nil {
		c.NewControlPlaneServer = defaults.NewControlPlaneServer
	}
	if c.BuildEmbeddingEncoder == nil {
		c.BuildEmbeddingEncoder = defaults.BuildEmbeddingEncoder
	}

	return c
}
