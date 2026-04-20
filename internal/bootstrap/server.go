package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

type controlPlaneShutdown interface {
	Shutdown(ctx context.Context) error
}

func serveUntilShutdown(server *http.Server, service controlPlaneShutdown) error {
	if server == nil {
		return errors.New("control plane server must not be nil")
	}
	if service == nil {
		return errors.New("control plane service must not be nil")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		runErr := normalizeControlPlaneServerRunError(err)
		serviceErr := shutdownControlPlaneService(service)
		return errors.Join(runErr, serviceErr)
	case <-ctx.Done():
		serviceErr := shutdownControlPlaneService(service)
		serverErr := shutdownControlPlaneServer(server)
		err := <-errCh
		runErr := normalizeControlPlaneServerRunError(err)
		return errors.Join(serviceErr, serverErr, runErr)
	}
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

	if err := service.Shutdown(shutdownCtx); err != nil {
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
