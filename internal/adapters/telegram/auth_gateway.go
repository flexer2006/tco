package telegram

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	gotdtelegram "github.com/gotd/td/telegram"
	tgauth "github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	onboardingapp "github.com/flexer2006/tco/internal/application/onboarding"
)

const (
	authOperationStart          = "start"
	authOperationVerifyCode     = "verify_code"
	authOperationVerifyPassword = "verify_password"

	fieldAPIID           = "api_id"
	reasonMustNotBeEmpty = "must not be empty"

	sessionDirMode          = 0o700
	telegramRPCUnauthorized = 401
	telegramRPCFloodWait    = 420
)

type (
	authGatewayRuntimeClientAdapter struct {
		client *gotdtelegram.Client
	}
	LiveAuthGatewayOption    func(*LiveAuthGateway)
	authGatewayRuntimeClient interface {
		Run(
			ctx context.Context,
			callback func(ctx context.Context, authClient authGatewayClient) error,
		) error
	}
	authGatewayRuntimeClientFactory func(appID int, appHash, sessionPath, proxyAddr string) authGatewayRuntimeClient
	authGatewayClient               interface {
		SendCode(
			ctx context.Context,
			phone string,
			options tgauth.SendCodeOptions,
		) (tg.AuthSentCodeClass, error)
		SignIn(ctx context.Context, phone, code, codeHash string) (*tg.AuthAuthorization, error)
		Password(ctx context.Context, password string) (*tg.AuthAuthorization, error)
	}
	LiveAuthGateway struct {
		sessionPath, proxyAddr, authorizedKey, phone, phoneCodeHash, configuredAppHash string
		newRuntimeClient                                                               authGatewayRuntimeClientFactory
		mu                                                                             sync.Mutex
		authorizedApp, configuredAppID                                                 int
		sendCodeOptions                                                                tgauth.SendCodeOptions
	}
)

func WithLiveAuthGatewayProxyAddr(proxyAddr string) LiveAuthGatewayOption {
	return func(gateway *LiveAuthGateway) {
		if gateway == nil {
			return
		}

		gateway.proxyAddr = strings.TrimSpace(proxyAddr)
	}
}

func WithLiveAuthGatewayAppCredentials(appID int, appHash string) LiveAuthGatewayOption {
	return func(gateway *LiveAuthGateway) {
		if gateway == nil {
			return
		}

		gateway.configuredAppID = appID
		gateway.configuredAppHash = strings.TrimSpace(appHash)
	}
}

func NewLiveAuthGateway(
	sessionPath string,
	options ...LiveAuthGatewayOption,
) (*LiveAuthGateway, error) {
	trimmedSessionPath := strings.TrimSpace(sessionPath)
	if trimmedSessionPath == "" {
		return nil, ErrAuthSessionPathEmpty
	}

	err := os.MkdirAll(filepath.Dir(trimmedSessionPath), sessionDirMode)
	if err != nil {
		return nil, fmt.Errorf("create telegram auth session directory: %w", err)
	}

	gateway := new(LiveAuthGateway{
		sessionPath:      trimmedSessionPath,
		newRuntimeClient: defaultAuthGatewayRuntimeClientFactory,
	})

	for _, option := range options {
		if option != nil {
			option(gateway)
		}
	}

	if gateway.newRuntimeClient == nil {
		return nil, ErrAuthRuntimeClientFactoryNil
	}

	return gateway, nil
}

func (g *LiveAuthGateway) Start(ctx context.Context, apiID, apiHash, phone string) error {
	if g == nil {
		return new(onboardingapp.OperationError{
			Kind:      onboardingapp.ErrorKindInternal,
			Operation: authOperationStart,
			Advice:    adviceForOnboardingErrorKind(onboardingapp.ErrorKindInternal),
			Err:       ErrAuthGatewayNil,
		})
	}

	parsedAPIID, err := parseAPIID(apiID)
	if err != nil {
		return err
	}

	trimmedAPIHash := strings.TrimSpace(apiHash)
	if trimmedAPIHash == "" {
		return onboardingapp.InvalidInputError{Field: "api_hash", Reason: reasonMustNotBeEmpty}
	}

	if g.configuredAppID > 0 && parsedAPIID != g.configuredAppID {
		return onboardingapp.InvalidInputError{
			Field:  fieldAPIID,
			Reason: "must match TELEGRAM_API_ID from environment",
		}
	}

	if g.configuredAppHash != "" && trimmedAPIHash != g.configuredAppHash {
		return onboardingapp.InvalidInputError{
			Field:  "api_hash",
			Reason: "must match TELEGRAM_API_HASH from environment",
		}
	}

	trimmedPhone := strings.TrimSpace(phone)
	if trimmedPhone == "" {
		return onboardingapp.InvalidInputError{Field: "phone", Reason: reasonMustNotBeEmpty}
	}

	runtimeClient := g.newRuntimeClient(parsedAPIID, trimmedAPIHash, g.sessionPath, g.proxyAddr)
	if runtimeClient == nil {
		return new(onboardingapp.OperationError{
			Kind:      onboardingapp.ErrorKindInternal,
			Operation: authOperationStart,
			Advice:    adviceForOnboardingErrorKind(onboardingapp.ErrorKindInternal),
			Err:       ErrAuthRuntimeClientNil,
		})
	}

	var phoneCodeHash string

	runErr := runtimeClient.Run(
		ctx,
		func(runCtx context.Context, authClient authGatewayClient) error {
			sentCode, sendErr := authClient.SendCode(runCtx, trimmedPhone, g.sendCodeOptions)
			if sendErr != nil {
				return sendErr
			}

			extractedHash, extractErr := extractPhoneCodeHash(sentCode)
			if extractErr != nil {
				return extractErr
			}

			phoneCodeHash = extractedHash

			return nil
		},
	)
	if runErr != nil {
		return mapTelegramAuthError(authOperationStart, runErr)
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	g.authorizedApp = parsedAPIID
	g.authorizedKey = trimmedAPIHash
	g.phone = trimmedPhone
	g.phoneCodeHash = phoneCodeHash

	return nil
}

func (g *LiveAuthGateway) VerifyCode(ctx context.Context, code string) error {
	if g == nil {
		return new(onboardingapp.OperationError{
			Kind:      onboardingapp.ErrorKindInternal,
			Operation: authOperationVerifyCode,
			Advice:    adviceForOnboardingErrorKind(onboardingapp.ErrorKindInternal),
			Err:       ErrAuthGatewayNil,
		})
	}

	trimmedCode := strings.TrimSpace(code)
	if trimmedCode == "" {
		return onboardingapp.InvalidInputError{Field: "code", Reason: reasonMustNotBeEmpty}
	}

	runtimeClient, phone, phoneCodeHash, err := g.runtimeClientLocked(authOperationVerifyCode)
	if err != nil {
		return err
	}

	runErr := runtimeClient.Run(
		ctx,
		func(runCtx context.Context, authClient authGatewayClient) error {
			_, signInErr := authClient.SignIn(runCtx, phone, trimmedCode, phoneCodeHash)

			return signInErr
		},
	)
	if runErr != nil {
		if errors.Is(runErr, tgauth.ErrPasswordAuthNeeded) {
			return onboardingapp.ErrPasswordRequired
		}

		return mapTelegramAuthError(authOperationVerifyCode, runErr)
	}

	g.mu.Lock()
	g.phoneCodeHash = ""
	g.mu.Unlock()

	return nil
}

func (g *LiveAuthGateway) VerifyPassword(ctx context.Context, password string) error {
	if g == nil {
		return new(onboardingapp.OperationError{
			Kind:      onboardingapp.ErrorKindInternal,
			Operation: authOperationVerifyPassword,
			Advice:    adviceForOnboardingErrorKind(onboardingapp.ErrorKindInternal),
			Err:       ErrAuthGatewayNil,
		})
	}

	trimmedPassword := strings.TrimSpace(password)
	if trimmedPassword == "" {
		return onboardingapp.InvalidInputError{Field: "password", Reason: reasonMustNotBeEmpty}
	}

	runtimeClient, _, _, err := g.runtimeClientLocked(authOperationVerifyPassword)
	if err != nil {
		return err
	}

	runErr := runtimeClient.Run(
		ctx,
		func(runCtx context.Context, authClient authGatewayClient) error {
			_, passwordErr := authClient.Password(runCtx, trimmedPassword)

			return passwordErr
		},
	)
	if runErr != nil {
		return mapTelegramAuthError(authOperationVerifyPassword, runErr)
	}

	return nil
}

func (g *LiveAuthGateway) Logout(ctx context.Context) error {
	if g == nil {
		return new(onboardingapp.OperationError{
			Kind:      onboardingapp.ErrorKindInternal,
			Operation: "logout",
			Advice:    adviceForOnboardingErrorKind(onboardingapp.ErrorKindInternal),
			Err:       ErrAuthGatewayNil,
		})
	}

	appID, appHash := g.sessionCredentials()
	if appID <= 0 || appHash == "" {
		return nil
	}

	runtimeClient := g.newRuntimeClient(appID, appHash, g.sessionPath, g.proxyAddr)
	if runtimeClient == nil {
		return new(onboardingapp.OperationError{
			Kind:      onboardingapp.ErrorKindInternal,
			Operation: "logout",
			Advice:    adviceForOnboardingErrorKind(onboardingapp.ErrorKindInternal),
			Err:       ErrAuthRuntimeClientNil,
		})
	}

	runErr := runtimeClient.Run(
		ctx,
		func(runCtx context.Context, authClient authGatewayClient) error {
			if statusClient, ok := authClient.(interface {
				Status(ctx context.Context) (*tgauth.Status, error)
			}); ok {
				status, statusErr := statusClient.Status(runCtx)
				if statusErr != nil {
					return statusErr
				}

				if status == nil || !status.Authorized {
					return nil
				}
			}

			if logoutClient, ok := authClient.(interface {
				LogOut(ctx context.Context) error
			}); ok {
				return logoutClient.LogOut(runCtx)
			}

			return ErrAuthClientNoLogout
		},
	)
	if runErr != nil {
		return mapTelegramAuthError("logout", runErr)
	}

	g.mu.Lock()
	g.authorizedApp = 0
	g.authorizedKey = ""
	g.phone = ""
	g.phoneCodeHash = ""
	g.mu.Unlock()

	return nil
}

func (g *LiveAuthGateway) Authorized(ctx context.Context) (bool, error) {
	if g == nil {
		return false, ErrAuthGatewayNil
	}

	appID, appHash := g.sessionCredentials()
	if appID <= 0 || appHash == "" {
		return false, nil
	}

	runtimeClient := g.newRuntimeClient(appID, appHash, g.sessionPath, g.proxyAddr)
	if runtimeClient == nil {
		return false, ErrAuthRuntimeClientNil
	}

	var authorized bool

	runErr := runtimeClient.Run(
		ctx,
		func(runCtx context.Context, authClient authGatewayClient) error {
			statusClient, ok := authClient.(interface {
				Status(ctx context.Context) (*tgauth.Status, error)
			})
			if !ok {
				return ErrAuthClientNoStatus
			}

			status, err := statusClient.Status(runCtx)
			if err != nil {
				return err
			}

			authorized = status != nil && status.Authorized

			return nil
		},
	)
	if runErr != nil {
		return false, mapTelegramAuthError("authorized", runErr)
	}

	return authorized, nil
}

func (g *LiveAuthGateway) sessionCredentials() (int, string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.authorizedApp > 0 && strings.TrimSpace(g.authorizedKey) != "" {
		return g.authorizedApp, g.authorizedKey
	}

	return g.configuredAppID, g.configuredAppHash
}

func (g *LiveAuthGateway) runtimeClientLocked(
	operation string,
) (authGatewayRuntimeClient, string, string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.authorizedApp <= 0 || strings.TrimSpace(g.authorizedKey) == "" {
		return nil, "", "", new(onboardingapp.OperationError{
			Kind:      onboardingapp.ErrorKindInternal,
			Operation: operation,
			Advice:    "start authentication before verifying code/password",
			Err:       ErrAuthContextNotInitialized,
		})
	}

	runtimeClient := g.newRuntimeClient(
		g.authorizedApp,
		g.authorizedKey,
		g.sessionPath,
		g.proxyAddr,
	)
	if runtimeClient == nil {
		return nil, "", "", new(onboardingapp.OperationError{
			Kind:      onboardingapp.ErrorKindInternal,
			Operation: operation,
			Advice:    adviceForOnboardingErrorKind(onboardingapp.ErrorKindInternal),
			Err:       ErrAuthRuntimeClientNil,
		})
	}

	if operation == authOperationVerifyCode && strings.TrimSpace(g.phoneCodeHash) == "" {
		return nil, "", "", new(onboardingapp.OperationError{
			Kind:      onboardingapp.ErrorKindInternal,
			Operation: operation,
			Advice:    "start authentication before submitting verification code",
			Err:       ErrMissingPhoneCodeHash,
		})
	}

	return runtimeClient, g.phone, g.phoneCodeHash, nil
}

func parseAPIID(apiID string) (int, error) {
	trimmedAPIID := strings.TrimSpace(apiID)
	if trimmedAPIID == "" {
		return 0, onboardingapp.InvalidInputError{Field: fieldAPIID, Reason: reasonMustNotBeEmpty}
	}

	parsedAPIID, err := strconv.Atoi(trimmedAPIID)
	if err != nil || parsedAPIID <= 0 {
		return 0, onboardingapp.InvalidInputError{
			Field:  fieldAPIID,
			Reason: "must be a positive integer",
		}
	}

	return parsedAPIID, nil
}

func extractPhoneCodeHash(sentCode tg.AuthSentCodeClass) (string, error) {
	if sentCode == nil {
		return "", ErrSendCodeNilResponse
	}

	switch typed := sentCode.(type) {
	default:
		return "", fmt.Errorf("%w: %T", ErrUnsupportedSendCodeResponse, sentCode)
	case *tg.AuthSentCode:
		codeHash := strings.TrimSpace(typed.GetPhoneCodeHash())
		if codeHash == "" {
			return "", ErrSendCodeEmptyPhoneCodeHash
		}

		return codeHash, nil
	case *tg.AuthSentCodePaymentRequired:
		codeHash := strings.TrimSpace(typed.GetPhoneCodeHash())
		if codeHash == "" {
			return "", ErrSendCodePaymentEmptyPhoneCodeHash
		}

		return codeHash, nil
	}
}

func mapTelegramAuthError(operation string, err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return new(onboardingapp.OperationError{
			Kind:      onboardingapp.ErrorKindTimeout,
			Operation: operation,
			Advice:    adviceForOnboardingErrorKind(onboardingapp.ErrorKindTimeout),
			Err:       err,
		})
	}

	if retryAfter, ok := tgerr.AsFloodWait(err); ok {
		return new(onboardingapp.OperationError{
			Kind:       onboardingapp.ErrorKindRateLimit,
			Operation:  operation,
			RetryAfter: retryAfter,
			Advice:     adviceForOnboardingErrorKind(onboardingapp.ErrorKindRateLimit),
			Err:        err,
		})
	}

	if isTelegramInvalidTargetError(err) {
		return new(onboardingapp.OperationError{
			Kind:      onboardingapp.ErrorKindInvalidTarget,
			Operation: operation,
			Advice:    adviceForOnboardingErrorKind(onboardingapp.ErrorKindInvalidTarget),
			Err:       err,
		})
	}

	if isTelegramAuthOperationError(err) {
		return new(onboardingapp.OperationError{
			Kind:      onboardingapp.ErrorKindAuth,
			Operation: operation,
			Advice:    adviceForOnboardingErrorKind(onboardingapp.ErrorKindAuth),
			Err:       err,
		})
	}

	if isNetworkError(err) {
		return new(onboardingapp.OperationError{
			Kind:      onboardingapp.ErrorKindNetwork,
			Operation: operation,
			Advice:    adviceForOnboardingErrorKind(onboardingapp.ErrorKindNetwork),
			Err:       err,
		})
	}

	return new(onboardingapp.OperationError{
		Kind:      onboardingapp.ErrorKindInternal,
		Operation: operation,
		Advice:    adviceForOnboardingErrorKind(onboardingapp.ErrorKindInternal),
		Err:       err,
	})
}

func isTelegramAuthOperationError(err error) bool {
	if err == nil {
		return false
	}

	if tgauth.IsUnauthorized(err) || tgerr.IsCode(err, telegramRPCUnauthorized) {
		return true
	}

	rpcErr, ok := tgerr.As(err)
	if !ok {
		return false
	}

	if rpcErr.IsOneOf(
		"PHONE_CODE_INVALID",
		"PHONE_CODE_EXPIRED",
		"PHONE_NUMBER_INVALID",
		"PHONE_NUMBER_UNOCCUPIED",
		"PHONE_NUMBER_BANNED",
		"PHONE_CODE_EMPTY",
		"PASSWORD_HASH_INVALID",
		"SESSION_PASSWORD_NEEDED",
		"AUTH_KEY_UNREGISTERED",
		"SESSION_EXPIRED",
		"SESSION_REVOKED",
		"API_ID_INVALID",
	) {
		return true
	}

	upperType := strings.ToUpper(strings.TrimSpace(rpcErr.Type))
	if strings.Contains(upperType, "PHONE_CODE") &&
		(strings.Contains(upperType, "INVALID") || strings.Contains(upperType, "EXPIRED")) {
		return true
	}

	if strings.Contains(upperType, "PASSWORD") && strings.Contains(upperType, "INVALID") {
		return true
	}

	return false
}

func isTelegramInvalidTargetError(err error) bool {
	rpcErr, ok := tgerr.As(err)
	if !ok {
		return false
	}

	if rpcErr.IsOneOf(
		"USERNAME_INVALID",
		"CHANNEL_INVALID",
		"CHAT_ID_INVALID",
		"PEER_ID_INVALID",
		"USER_ID_INVALID",
	) {
		return true
	}

	upperType := strings.ToUpper(strings.TrimSpace(rpcErr.Type))

	return strings.Contains(upperType, "PEER") && strings.Contains(upperType, "INVALID")
}

func adviceForOnboardingErrorKind(kind onboardingapp.ErrorKind) string {
	switch kind {
	default:
		return "unexpected Telegram auth error; retry and inspect logs if it persists"
	case onboardingapp.ErrorKindAuth:
		return "verify Telegram credentials/code/password and retry"
	case onboardingapp.ErrorKindNetwork:
		return "check network connectivity to Telegram and retry"
	case onboardingapp.ErrorKindRateLimit:
		return "Telegram returned FLOOD_WAIT; retry after the provided delay"
	case onboardingapp.ErrorKindInvalidTarget:
		return "check the target identifier format and retry"
	case onboardingapp.ErrorKindTimeout:
		return "operation timed out while waiting for Telegram; retry"
	}
}

func (a authGatewayRuntimeClientAdapter) Run(
	ctx context.Context,
	callback func(context.Context, authGatewayClient) error,
) error {
	if a.client == nil {
		return ErrGotdAuthClientNil
	}

	return runTelegramClient(a.client, ctx, func(runCtx context.Context) error {
		return callback(
			runCtx,
			authGatewayClientAdapter{auth: a.client.Auth(), api: a.client.API()},
		)
	})
}

func defaultAuthGatewayRuntimeClientFactory(
	appID int,
	appHash, sessionPath, proxyAddr string,
) authGatewayRuntimeClient {
	options, err := telegramClientOptions(sessionPath, proxyAddr)
	if err != nil {
		return failedAuthRuntimeClient{err: err}
	}

	return authGatewayRuntimeClientAdapter{client: gotdtelegram.NewClient(appID, appHash, options)}
}

type authGatewayClientAdapter struct {
	auth *tgauth.Client
	api  *tg.Client
}

func (a authGatewayClientAdapter) SendCode(
	ctx context.Context,
	phone string,
	options tgauth.SendCodeOptions,
) (tg.AuthSentCodeClass, error) {
	sentCode, err := a.auth.SendCode(ctx, phone, options)
	if err != nil {
		return nil, fmt.Errorf("telegram auth send code: %w", err)
	}

	return sentCode, nil
}

func (a authGatewayClientAdapter) SignIn(
	ctx context.Context,
	phone, code, codeHash string,
) (*tg.AuthAuthorization, error) {
	authorization, err := a.auth.SignIn(ctx, phone, code, codeHash)
	if err != nil {
		return nil, fmt.Errorf("telegram auth sign in: %w", err)
	}

	return authorization, nil
}

func (a authGatewayClientAdapter) Password(
	ctx context.Context,
	password string,
) (*tg.AuthAuthorization, error) {
	authorization, err := a.auth.Password(ctx, password)
	if err != nil {
		return nil, fmt.Errorf("telegram auth password: %w", err)
	}

	return authorization, nil
}

func (a authGatewayClientAdapter) Status(ctx context.Context) (*tgauth.Status, error) {
	status, err := a.auth.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("telegram auth status: %w", err)
	}

	return status, nil
}

func (a authGatewayClientAdapter) LogOut(ctx context.Context) error {
	_, err := a.api.AuthLogOut(ctx)
	if err != nil {
		return fmt.Errorf("telegram auth logout: %w", err)
	}

	return nil
}
