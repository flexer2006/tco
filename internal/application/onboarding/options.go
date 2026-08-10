package onboarding

import (
	"context"
	"strings"
)

type (
	AuthBackend interface {
		Start(ctx context.Context, apiID, apiHash, phone string) error
		VerifyCode(ctx context.Context, code string) error
		VerifyPassword(ctx context.Context, password string) error
		Logout(ctx context.Context) error
		Authorized(ctx context.Context) (bool, error)
	}
	ServiceOption func(service *Onboarding) error
)

func WithAuthBackend(backend AuthBackend) ServiceOption {
	return func(service *Onboarding) error {
		if service == nil {
			return ErrOnboardingServiceNil
		}

		if backend == nil {
			return InvalidInputError{Field: "auth_backend", Reason: "must not be nil"}
		}

		service.authBackend = backend

		return nil
	}
}

func WithExpectedTelegramCredentials(apiID, apiHash string) ServiceOption {
	return func(service *Onboarding) error {
		if service == nil {
			return ErrOnboardingServiceNil
		}

		service.expectedAPIID = strings.TrimSpace(apiID)
		service.expectedAPIHash = strings.TrimSpace(apiHash)

		return nil
	}
}
