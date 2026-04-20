package application

import (
	"context"
	"errors"
)

type (
	AuthBackend interface {
		Start(ctx context.Context, apiID, apiHash, phone string) error
		VerifyCode(ctx context.Context, code string) error
		VerifyPassword(ctx context.Context, password string) error
	}
	ServiceOption func(service *Service2) error
)

func WithAuthBackend(backend AuthBackend) ServiceOption {
	return func(service *Service2) error {
		if service == nil {
			return errors.New("onboarding service must not be nil")
		}
		if backend == nil {
			return InvalidInputError{Field: "auth_backend", Reason: "must not be nil"}
		}
		service.authBackend = backend
		return nil
	}
}
