package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/flexer2006/tco/internal/adapters/embedding"
	"github.com/flexer2006/tco/internal/adapters/telegram"
	"github.com/flexer2006/tco/internal/application/onboarding"
)

const (
	runtimeProfileReal          = "real"
	telegramSourceModeLive      = "live"
	controlPlaneShutdownTimeout = 5 * time.Second
)

func Serve() error {
	return serve(newContainer())
}

func serve(cont container) error {
	cont = cont.withDefaults()

	cfg, err := cont.LoadConfig()
	if err != nil {
		return err
	}

	orchestrator, err := cont.orchestrator(cfg)
	if err != nil {
		return err
	}

	service, err := cont.NewControlPlaneService(orchestrator, cfg.TelegramChatID)
	if err != nil {
		return fmt.Errorf("build control plane service: %w", err)
	}

	appID, err := telegramAppID(cfg.TelegramAPIID)
	if err != nil {
		return err
	}

	options := []telegram.LiveAuthGatewayOption{
		telegram.WithLiveAuthGatewayAppCredentials(appID, cfg.TelegramAPIHash),
	}
	if proxy := strings.TrimSpace(cfg.TelegramProxyAddr); proxy != "" {
		options = append(options, telegram.WithLiveAuthGatewayProxyAddr(proxy))
	}

	authGateway, gatewayErr := cont.NewLiveAuthGateway(cfg.TelegramSessionPath, options...)
	if gatewayErr != nil {
		return fmt.Errorf("build telegram auth gateway: %w", gatewayErr)
	}

	onboardingService, err := cont.NewOnboardingService(
		cfg.TelegramSessionPath,
		onboarding.WithAuthBackend(authGateway),
		onboarding.WithExpectedTelegramCredentials(cfg.TelegramAPIID, cfg.TelegramAPIHash),
	)
	if err != nil {
		return fmt.Errorf("build onboarding service: %w", err)
	}

	server, err := cont.NewControlPlaneServer(
		cfg.HTTPBind,
		cfg.HTTPPort,
		service,
		onboardingService,
		cfg.ControlPlaneToken,
	)
	if err != nil {
		return fmt.Errorf("build control plane server: %w", err)
	}

	return serveUntilShutdown(server, service, cfg.TLSCertFile, cfg.TLSKeyFile)
}

type controlPlaneShutdown interface {
	Shutdown(ctx context.Context) error
}

func serveUntilShutdown(
	server *http.Server,
	service controlPlaneShutdown,
	tlsCertFile, tlsKeyFile string,
) error {
	if server == nil {
		return ErrControlPlaneServerNil
	}

	if service == nil {
		return ErrControlPlaneServiceNil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- listenAndServeControlPlane(server, tlsCertFile, tlsKeyFile)
	}()

	select {
	case err := <-errCh:
		runErr := normalizeControlPlaneServerRunError(err)
		serviceErr := shutdownControlPlaneService(service)

		embedding.CloseCachedONNXSessions()

		return errors.Join(runErr, serviceErr)
	case <-ctx.Done():
		serviceErr := shutdownControlPlaneService(service)
		serverErr := shutdownControlPlaneServer(server)

		embedding.CloseCachedONNXSessions()

		err := <-errCh
		runErr := normalizeControlPlaneServerRunError(err)

		return errors.Join(serviceErr, serverErr, runErr)
	}
}

func listenAndServeControlPlane(server *http.Server, tlsCertFile, tlsKeyFile string) error {
	cert := strings.TrimSpace(tlsCertFile)
	key := strings.TrimSpace(tlsKeyFile)

	if cert != "" && key != "" {
		return server.ListenAndServeTLS(cert, key)
	}

	return server.ListenAndServe()
}

func normalizeControlPlaneServerRunError(err error) error {
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}

	return fmt.Errorf("run control plane server: %w", err)
}

func shutdownControlPlaneService(service controlPlaneShutdown) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), controlPlaneShutdownTimeout)
	defer cancel()

	err := service.Shutdown(shutdownCtx)
	if err != nil {
		return fmt.Errorf("shutdown control plane service: %w", err)
	}

	return nil
}

func shutdownControlPlaneServer(server *http.Server) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), controlPlaneShutdownTimeout)
	defer cancel()

	err := server.Shutdown(shutdownCtx)
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}

	return fmt.Errorf("shutdown control plane server: %w", err)
}
