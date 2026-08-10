package httpcontrol

import "errors"

var (
	ErrHTTPBindEmpty          = errors.New("http bind must not be empty")
	ErrHTTPPortOutOfRange     = errors.New("http port must be in range")
	ErrControlPlaneServiceNil = errors.New("control plane service must not be nil")
	ErrOnboardingServiceNil   = errors.New("onboarding service must not be nil")
)
