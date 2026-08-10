package telegram

import "errors"

var (
	ErrAuthSessionPathEmpty              = errors.New("telegram auth session path must not be empty")
	ErrAuthRuntimeClientFactoryNil       = errors.New("telegram auth runtime client factory must not be nil")
	ErrAuthGatewayNil                    = errors.New("telegram auth gateway is nil")
	ErrAuthRuntimeClientNil              = errors.New("telegram auth runtime client is nil")
	ErrAuthClientNoLogout                = errors.New("telegram auth client does not support logout")
	ErrAuthClientNoStatus                = errors.New("telegram auth client does not support status checks")
	ErrAuthContextNotInitialized         = errors.New("telegram auth context is not initialized")
	ErrMissingPhoneCodeHash              = errors.New("missing phone code hash from start operation")
	ErrSendCodeNilResponse               = errors.New("telegram send code returned nil response")
	ErrUnsupportedSendCodeResponse       = errors.New("unsupported telegram send code response type")
	ErrSendCodeEmptyPhoneCodeHash        = errors.New("telegram send code returned empty phone code hash")
	ErrSendCodePaymentEmptyPhoneCodeHash = errors.New(
		"telegram send code payment-required response returned empty phone code hash",
	)
	ErrGotdAuthClientNil                 = errors.New("gotd auth client must not be nil")
	ErrLiveHistoryPageSizeNotPositive    = errors.New("live history page size must be positive")
	ErrGotdClientNil                     = errors.New("gotd client must not be nil")
	ErrAppIDNotPositive                  = errors.New("telegram app id must be positive")
	ErrAppHashEmpty                      = errors.New("telegram app hash must not be empty")
	ErrSessionPathEmpty                  = errors.New("telegram session path must not be empty")
	ErrLiveRuntimeClientFactoryNil       = errors.New("telegram live runtime client factory must not be nil")
	ErrLiveHistoryPageSizeMustBePositive = errors.New("telegram live history page size must be positive")
	ErrLiveHistoryMaxMessagesNotPositive = errors.New(
		"telegram live history max messages must be positive",
	)
	ErrLiveSourceNil               = errors.New("telegram live source must not be nil")
	ErrContextNil                  = errors.New("context must not be nil")
	ErrSourceChatEmpty             = errors.New("source chat must not be empty")
	ErrUsernameEmpty               = errors.New("username must not be empty")
	ErrChatIDEmpty                 = errors.New("chat id must not be empty")
	ErrChatIDNotPositive           = errors.New("chat id must be positive")
	ErrChannelPeerIDPrefix         = errors.New("channel peer id must start with -100")
	ErrChannelPeerIDSuffix         = errors.New("channel peer id must contain channel id suffix")
	ErrChannelIDNotPositive        = errors.New("channel id must be positive")
	ErrInvalidAccessHashTargetForm = errors.New("invalid access-hash target form")
	ErrValueNotPositive            = errors.New("value must be positive")
	ErrUnsupportedLiveTargetKind   = errors.New("unsupported live target kind")
	ErrResolveUsernameNilResponse  = errors.New("resolve username returned nil response")
	ErrUnsupportedResolvedPeerType = errors.New("unsupported resolved peer type")
	ErrResolvedUserNoAccessHash    = errors.New("resolved user has no access hash")
	ErrResolvedUserNotFound        = errors.New("resolved user not found")
	ErrResolvedChannelNoAccessHash = errors.New("resolved channel has no access hash")
	ErrResolvedChannelNotFound     = errors.New("resolved channel not found")
)
