package domain

import "time"

type State string

const (
	StateAwaitingCredentials State = "awaiting_credentials"
	StateAuthCodeRequested   State = "auth_code_requested"
	StateAwaiting2FA         State = "awaiting_2fa_if_required"
	StateReady               State = "ready"
	StateDegradedOrFailed    State = "degraded_or_failed"
)

type (
	Snapshot struct {
		UpdatedAt      time.Time `json:"updated_at,omitzero"`
		State          State     `json:"state"`
		RuntimeProfile string    `json:"runtime_profile"`
		SessionPath    string    `json:"session_path"`
		Phone          string    `json:"phone,omitzero"`
		Reason         string    `json:"reason,omitzero"`
	}
	Readiness struct {
		State  State  `json:"state"`
		Reason string `json:"reason,omitzero"`
		Ready  bool   `json:"ready"`
	}
)
