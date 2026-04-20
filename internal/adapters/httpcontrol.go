package adapters

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"github.com/flexer2006/tco/internal/application"
	"github.com/flexer2006/tco/internal/domain"
	"time"
)

type (
	Service interface {
		Readiness() application.Readiness
		Status() application.Status
		TriggerRun(ctx context.Context) error
	}
	OnboardingService interface {
		Snapshot() domain.Snapshot
		Readiness() domain.Readiness
		Start(ctx context.Context, apiID, apiHash, phone string) error
		VerifyCode(ctx context.Context, code string) error
		VerifyPassword(ctx context.Context, password string) error
		Logout(ctx context.Context) error
	}
)

func NewServer(bind string, port int, service Service, onboarding OnboardingService) (*http.Server, error) {
	bind = strings.TrimSpace(bind)
	if bind == "" {
		return nil, errors.New("http bind must not be empty")
	}
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("http port must be in range 1..65535, got %d", port)
	}
	handler, err := newHandler(service, onboarding)
	if err != nil {
		return nil, err
	}
	addr := net.JoinHostPort(bind, strconv.Itoa(port))
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}, nil
}

func newHandler(service Service, onboarding OnboardingService) (http.Handler, error) {
	if service == nil {
		return nil, errors.New("control plane service must not be nil")
	}
	if onboarding == nil {
		return nil, errors.New("onboarding service must not be nil")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		pipelineReadiness := service.Readiness()
		onboardingReadiness := onboarding.Readiness()
		if pipelineReadiness.Ready && onboardingReadiness.Ready {
			writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
			return
		}
		reason := pipelineReadiness.Reason
		if pipelineReadiness.Ready {
			reason = onboardingReadiness.Reason
		}
		reason = strings.TrimSpace(reason)
		if reason == "" {
			reason = "service is not ready"
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not_ready",
			"reason": reason,
		})
	})
	mux.HandleFunc("GET /auth", func(w http.ResponseWriter, _ *http.Request) {
		csrfToken, err := generateCSRFToken()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"status": "error", "error": "csrf_token_generation_failed"})
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     csrfCookieName,
			Value:    csrfToken,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		})
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(authPageHTML(onboarding.Snapshot(), csrfToken)))
	})
	mux.HandleFunc("GET /auth/state", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"snapshot":  onboarding.Snapshot(),
			"readiness": onboarding.Readiness(),
		})
	})
	mux.HandleFunc("POST /auth/start", func(w http.ResponseWriter, r *http.Request) {
		if err := parseAndValidateCSRF(r); err != nil {
			writeAuthPostValidationError(w, err)
			return
		}
		err := onboarding.Start(r.Context(), r.FormValue("api_id"), r.FormValue("api_hash"), r.FormValue("phone"))
		writeOnboardingStatus(w, onboarding, err)
	})
	mux.HandleFunc("POST /auth/verify-code", func(w http.ResponseWriter, r *http.Request) {
		if err := parseAndValidateCSRF(r); err != nil {
			writeAuthPostValidationError(w, err)
			return
		}
		err := onboarding.VerifyCode(r.Context(), r.FormValue("code"))
		writeOnboardingStatus(w, onboarding, err)
	})
	mux.HandleFunc("POST /auth/verify-password", func(w http.ResponseWriter, r *http.Request) {
		if err := parseAndValidateCSRF(r); err != nil {
			writeAuthPostValidationError(w, err)
			return
		}
		err := onboarding.VerifyPassword(r.Context(), r.FormValue("password"))
		writeOnboardingStatus(w, onboarding, err)
	})
	mux.HandleFunc("POST /auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if err := parseAndValidateCSRF(r); err != nil {
			writeAuthPostValidationError(w, err)
			return
		}
		err := onboarding.Logout(r.Context())
		writeOnboardingStatus(w, onboarding, err)
	})
	mux.HandleFunc("POST /pipeline/run", func(w http.ResponseWriter, r *http.Request) {
		err := service.TriggerRun(r.Context())
		status := service.Status()
		switch {
		case err == nil:
			writeJSON(w, http.StatusAccepted, map[string]any{
				"status":   "accepted",
				"pipeline": status,
			})
		case errors.Is(err, application.ErrRunInProgress):
			writeJSON(w, http.StatusConflict, map[string]any{
				"status":   "conflict",
				"error":    err.Error(),
				"pipeline": status,
			})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"status": "error",
				"error":  err.Error(),
			})
		}
	})
	mux.HandleFunc("GET /pipeline/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"pipeline": service.Status(),
		})
	})
	return mux, nil
}
