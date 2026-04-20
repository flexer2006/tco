package bootstrap

import (
	"fmt"
	"strings"
	"github.com/flexer2006/tco/internal/adapters"
	"github.com/flexer2006/tco/internal/application"
	"time"
)

const (
	runtimeProfileReal          = "real"
	telegramSourceModeLive      = "live"
	controlPlaneShutdownTimeout = 5 * time.Second
)

func Serve() error {
	return serveWithDeps(defaultDeps())
}

func serveWithDeps(deps deps) error {
	deps = deps.withDefaults()
	cfg, err := deps.LoadConfig()
	if err != nil {
		return err
	}
	orchestrator, err := buildOrchestratorWithDeps(cfg, deps)
	if err != nil {
		return err
	}
	service, err := deps.NewControlPlaneService(orchestrator, cfg.TelegramChatID)
	if err != nil {
		return fmt.Errorf("build control plane service: %w", err)
	}
	options := make([]adapters.LiveAuthGatewayOption, 0, 1)
	if strings.TrimSpace(cfg.TelegramProxyAddr) != "" {
		options = append(options, adapters.WithLiveAuthGatewayProxyAddr(cfg.TelegramProxyAddr))
	}
	authGateway, gatewayErr := deps.NewLiveAuthGateway(cfg.TelegramSessionPath, options...)
	if gatewayErr != nil {
		return fmt.Errorf("build telegram auth gateway: %w", gatewayErr)
	}
	onboardingService, err := deps.NewOnboardingService(cfg.TelegramSessionPath, application.WithAuthBackend(authGateway))
	if err != nil {
		return fmt.Errorf("build onboarding service: %w", err)
	}
	server, err := deps.NewControlPlaneServer(cfg.HTTPBind, cfg.HTTPPort, service, onboardingService)
	if err != nil {
		return fmt.Errorf("build control plane server: %w", err)
	}
	return serveUntilShutdown(server, service)
}
