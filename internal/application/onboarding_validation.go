package application

import (
	"context"
	"slices"
	"strings"
	"github.com/flexer2006/tco/internal/domain"
)

func ensureContext(ctx context.Context) error {
	if ctx == nil {
		return InvalidInputError{Field: "context", Reason: "must not be nil"}
	}
	return nil
}

func ensureNonEmpty(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return InvalidInputError{Field: field, Reason: "must not be empty"}
	}
	return nil
}

func ensureTransition(action string, current domain.State, allowed ...domain.State) error {
	if slices.Contains(allowed, current) {
		return nil
	}
	return invalidTransitionError{
		Action:  action,
		From:    current,
		Allowed: slices.Clone(allowed),
	}
}
