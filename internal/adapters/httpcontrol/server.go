package httpcontrol

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/flexer2006/tco/internal/application"
	"github.com/flexer2006/tco/internal/domain"
)

const (
	httpReadHeaderTimeout = 5 * time.Second
	httpReadTimeout       = 15 * time.Second
	httpWriteTimeout      = 30 * time.Second
	httpIdleTimeout       = 60 * time.Second
	httpMaxHeaderBytes    = 1 << 20
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
	ServerOptions struct {
		ControlPlaneToken string
	}
)

func NewServer(
	bind string,
	port int,
	service Service,
	onboarding OnboardingService,
	options ...ServerOptions,
) (*http.Server, error) {
	bind = strings.TrimSpace(bind)
	if bind == "" {
		return nil, ErrHTTPBindEmpty
	}

	const maxTCPPort = 65535

	if port <= 0 || port > maxTCPPort {
		return nil, fmt.Errorf("%w: 1..%d, got %d", ErrHTTPPortOutOfRange, maxTCPPort, port)
	}

	var opts ServerOptions
	if len(options) > 0 {
		opts = options[0]
	}

	handler, err := newHandler(service, onboarding, opts.ControlPlaneToken)
	if err != nil {
		return nil, err
	}

	addr := net.JoinHostPort(bind, strconv.Itoa(port))

	return new(http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: httpReadHeaderTimeout,
		ReadTimeout:       httpReadTimeout,
		WriteTimeout:      httpWriteTimeout,
		IdleTimeout:       httpIdleTimeout,
		MaxHeaderBytes:    httpMaxHeaderBytes,
	}), nil
}

func newHandler(
	service Service,
	onboarding OnboardingService,
	controlToken string,
) (http.Handler, error) {
	if service == nil {
		return nil, ErrControlPlaneServiceNil
	}

	if onboarding == nil {
		return nil, ErrOnboardingServiceNil
	}

	authn := newControlPlaneAuthn(controlToken)
	mux := http.NewServeMux()
	registerHealthRoutes(mux, service, onboarding)
	registerAuthRoutes(mux, authn, onboarding)
	registerPipelineRoutes(mux, authn, service)

	return mux, nil
}

func registerHealthRoutes(mux *http.ServeMux, service Service, onboarding OnboardingService) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{jsonKeyStatus: statusOK})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		pipelineReadiness := service.Readiness()

		onboardingReadiness := onboarding.Readiness()
		if pipelineReadiness.Ready && onboardingReadiness.Ready {
			writeJSON(w, http.StatusOK, map[string]string{jsonKeyStatus: statusReady})

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
			jsonKeyStatus: statusNotReady,
			jsonKeyReason: reason,
		})
	})
}

func registerAuthRoutes(
	mux *http.ServeMux,
	authn *controlPlaneAuthn,
	onboarding OnboardingService,
) {
	mux.HandleFunc(
		"GET /auth",
		requireControlToken(authn, handleAuthPage(onboarding)),
	)
	mux.HandleFunc(
		"GET /auth/state",
		requireControlToken(authn, func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, map[string]any{
				"snapshot":  publicAuthSnapshot(onboarding.Snapshot()),
				"readiness": publicAuthReadiness(onboarding.Readiness()),
			})
		}),
	)
	mux.HandleFunc(
		"POST /auth/start",
		requireControlToken(authn, handleAuthStart(authn, onboarding)),
	)
	mux.HandleFunc(
		"POST /auth/verify-code",
		requireControlToken(authn, handleAuthVerifyCode(authn, onboarding)),
	)
	mux.HandleFunc(
		"POST /auth/verify-password",
		requireControlToken(authn, handleAuthVerifyPassword(authn, onboarding)),
	)
	mux.HandleFunc(
		"POST /auth/logout",
		requireControlToken(authn, handleAuthLogout(onboarding)),
	)
}

func handleAuthPage(onboarding OnboardingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		csrfToken, err := generateCSRFToken()
		if err != nil {
			writeJSON(
				w,
				http.StatusInternalServerError,
				map[string]any{jsonKeyStatus: statusError, jsonKeyError: "csrf_token_generation_failed"},
			)

			return
		}

		http.SetCookie(w, new(http.Cookie{
			Name:     csrfCookieName,
			Value:    csrfToken,
			Path:     "/",
			HttpOnly: true,
			Secure:   r.TLS != nil,
			SameSite: http.SameSiteStrictMode,
		}))
		setSecurityHeaders(w)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		snapshot := onboarding.Snapshot()
		snapshot.SessionPath = "" // never embed filesystem paths in HTML
		snapshot.Reason = publicAuthReason(snapshot.Reason)
		_, _ = w.Write([]byte(authPageHTML(snapshot, csrfToken)))
	}
}

func handleAuthStart(authn *controlPlaneAuthn, onboarding OnboardingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !authn.allowAuthAttempt(r) {
			writeJSON(
				w,
				http.StatusTooManyRequests,
				map[string]any{jsonKeyStatus: statusRateLimited, jsonKeyError: errTooManyAuthAttempts},
			)

			return
		}

		err := parseAndValidateCSRF(r)
		if err != nil {
			writeAuthPostValidationError(w, err)

			return
		}

		err = onboarding.Start(
			r.Context(),
			r.FormValue("api_id"),
			r.FormValue("api_hash"),
			r.FormValue("phone"),
		)
		writeOnboardingStatus(w, onboarding, err)
	}
}

func handleAuthVerifyCode(authn *controlPlaneAuthn, onboarding OnboardingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !authn.allowAuthAttempt(r) {
			writeJSON(
				w,
				http.StatusTooManyRequests,
				map[string]any{jsonKeyStatus: statusRateLimited, jsonKeyError: errTooManyAuthAttempts},
			)

			return
		}

		err := parseAndValidateCSRF(r)
		if err != nil {
			writeAuthPostValidationError(w, err)

			return
		}

		err = onboarding.VerifyCode(r.Context(), r.FormValue("code"))
		writeOnboardingStatus(w, onboarding, err)
	}
}

func handleAuthVerifyPassword(
	authn *controlPlaneAuthn,
	onboarding OnboardingService,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !authn.allowAuthAttempt(r) {
			writeJSON(
				w,
				http.StatusTooManyRequests,
				map[string]any{jsonKeyStatus: statusRateLimited, jsonKeyError: errTooManyAuthAttempts},
			)

			return
		}

		err := parseAndValidateCSRF(r)
		if err != nil {
			writeAuthPostValidationError(w, err)

			return
		}

		err = onboarding.VerifyPassword(r.Context(), r.FormValue("password"))
		writeOnboardingStatus(w, onboarding, err)
	}
}

func handleAuthLogout(onboarding OnboardingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := parseAndValidateCSRF(r)
		if err != nil {
			writeAuthPostValidationError(w, err)

			return
		}

		err = onboarding.Logout(r.Context())
		writeOnboardingStatus(w, onboarding, err)
	}
}

func registerPipelineRoutes(mux *http.ServeMux, authn *controlPlaneAuthn, service Service) {
	mux.HandleFunc(
		"POST /pipeline/run",
		requireControlToken(authn, handlePipelineRun(service)),
	)
	mux.HandleFunc(
		"GET /pipeline/status",
		requireControlToken(authn, handlePipelineStatus(service)),
	)
}

func handlePipelineRun(service Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := parseAndValidateCSRF(r)
		if err != nil {
			writeJSON(
				w,
				http.StatusForbidden,
				map[string]any{jsonKeyStatus: statusForbidden, jsonKeyError: "invalid csrf token"},
			)

			return
		}

		err = service.TriggerRun(r.Context())
		status := service.Status()

		switch {
		case err == nil:
			writeJSON(w, http.StatusAccepted, map[string]any{
				jsonKeyStatus:   statusAccepted,
				jsonKeyPipeline: status,
			})
		case errors.Is(err, application.ErrRunInProgress):
			writeJSON(w, http.StatusConflict, map[string]any{
				jsonKeyStatus:   statusConflict,
				jsonKeyError:    publicErrorMessage(err),
				jsonKeyPipeline: status,
			})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				jsonKeyStatus: statusError,
				jsonKeyError:  publicErrorMessage(err),
			})
		}
	}
}

func handlePipelineStatus(service Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, err := ensureCSRFCookie(w, r)
		if err != nil {
			writeJSON(
				w,
				http.StatusInternalServerError,
				map[string]any{jsonKeyStatus: statusError, jsonKeyError: "csrf_token_generation_failed"},
			)

			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			jsonKeyPipeline: service.Status(),
		})
	}
}

func publicErrorMessage(err error) string {
	if err == nil {
		return "unknown error"
	}

	switch {
	case errors.Is(err, application.ErrRunInProgress):
		return application.ErrRunInProgress.Error()
	case errors.Is(err, application.ErrInvalidInput):
		return "invalid input"
	case errors.Is(err, context.Canceled):
		return "request canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "request timed out"
	default:
		return "internal error"
	}
}

func publicAuthSnapshot(snapshot domain.Snapshot) map[string]any {
	phone := snapshot.Phone
	if phone != "" && len(phone) > 4 {
		phone = "***" + phone[len(phone)-4:]
	}

	return map[string]any{
		"state":           snapshot.State,
		"runtime_profile": snapshot.RuntimeProfile,
		"updated_at":      snapshot.UpdatedAt,
		"phone":           phone,
		jsonKeyReason:     publicAuthReason(snapshot.Reason),
	}
}

func publicAuthReadiness(readiness domain.Readiness) map[string]any {
	return map[string]any{
		"ready":       readiness.Ready,
		"state":       readiness.State,
		jsonKeyReason: publicAuthReason(readiness.Reason),
	}
}

func publicAuthReason(reason string) string {
	reason = strings.TrimSpace(reason)
	switch {
	case reason == "":
		return ""
	case strings.Contains(reason, "credentials"):
		return "telegram credentials are required"
	case strings.Contains(reason, "verification code"):
		return "telegram verification code is required"
	case strings.Contains(reason, "cloud password"):
		return "telegram cloud password is required"
	case strings.Contains(reason, "not authorized"):
		return "telegram session is not authorized"
	default:
		return "onboarding is not ready"
	}
}
