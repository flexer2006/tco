package adapters

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	onboardingapp "github.com/flexer2006/tco/internal/application"

	gotdtelegram "github.com/gotd/td/telegram"
	tgauth "github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

const (
	authOperationStart          = "start"
	authOperationVerifyCode     = "verify_code"
	authOperationVerifyPassword = "verify_password"
)

type (
	authGatewayRuntimeClientAdapter struct {
		client *gotdtelegram.Client
	}
	LiveAuthGatewayOption    func(*LiveAuthGateway)
	authGatewayRuntimeClient interface {
		Run(ctx context.Context, callback func(ctx context.Context, authClient authGatewayClient) error) error
	}
	authGatewayRuntimeClientFactory func(appID int, appHash, sessionPath, proxyAddr string) authGatewayRuntimeClient
	authGatewayClient               interface {
		SendCode(ctx context.Context, phone string, options tgauth.SendCodeOptions) (tg.AuthSentCodeClass, error)
		SignIn(ctx context.Context, phone, code, codeHash string) (*tg.AuthAuthorization, error)
		Password(ctx context.Context, password string) (*tg.AuthAuthorization, error)
	}
	LiveAuthGateway struct {
		sessionPath, proxyAddr, authorizedKey, phone, phoneCodeHash string
		newRuntimeClient                                 authGatewayRuntimeClientFactory
		mu                                               sync.Mutex
		authorizedApp                                    int
		sendCodeOptions                                  tgauth.SendCodeOptions
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

func NewLiveAuthGateway(sessionPath string, options ...LiveAuthGatewayOption) (*LiveAuthGateway, error) {
	trimmedSessionPath := strings.TrimSpace(sessionPath)
	if trimmedSessionPath == "" {
		return nil, errors.New("telegram auth session path must not be empty")
	}
	if err := os.MkdirAll(filepath.Dir(trimmedSessionPath), 0o700); err != nil {
		return nil, fmt.Errorf("create telegram auth session directory: %w", err)
	}
	gateway := &LiveAuthGateway{
		sessionPath:      trimmedSessionPath,
		newRuntimeClient: defaultAuthGatewayRuntimeClientFactory,
	}
	for _, option := range options {
		if option != nil {
			option(gateway)
		}
	}
	if gateway.newRuntimeClient == nil {
		return nil, errors.New("telegram auth runtime client factory must not be nil")
	}
	return gateway, nil
}

func (g *LiveAuthGateway) Start(ctx context.Context, apiID, apiHash, phone string) error {
	if g == nil {
		return &onboardingapp.OperationError{
			Kind:      onboardingapp.ErrorKindInternal,
			Operation: authOperationStart,
			Advice:    adviceForOnboardingErrorKind(onboardingapp.ErrorKindInternal),
			Err:       errors.New("telegram auth gateway is nil"),
		}
	}
	parsedAPIID, err := parseAPIID(apiID)
	if err != nil {
		return err
	}
	trimmedAPIHash := strings.TrimSpace(apiHash)
	if trimmedAPIHash == "" {
		return onboardingapp.InvalidInputError{Field: "api_hash", Reason: "must not be empty"}
	}
	trimmedPhone := strings.TrimSpace(phone)
	if trimmedPhone == "" {
		return onboardingapp.InvalidInputError{Field: "phone", Reason: "must not be empty"}
	}
	runtimeClient := g.newRuntimeClient(parsedAPIID, trimmedAPIHash, g.sessionPath, g.proxyAddr)
	if runtimeClient == nil {
		return &onboardingapp.OperationError{
			Kind:      onboardingapp.ErrorKindInternal,
			Operation: authOperationStart,
			Advice:    adviceForOnboardingErrorKind(onboardingapp.ErrorKindInternal),
			Err:       errors.New("telegram auth runtime client is nil"),
		}
	}
	var phoneCodeHash string
	runErr := runtimeClient.Run(ctx, func(runCtx context.Context, authClient authGatewayClient) error {
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
	})
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
		return &onboardingapp.OperationError{
			Kind:      onboardingapp.ErrorKindInternal,
			Operation: authOperationVerifyCode,
			Advice:    adviceForOnboardingErrorKind(onboardingapp.ErrorKindInternal),
			Err:       errors.New("telegram auth gateway is nil"),
		}
	}
	trimmedCode := strings.TrimSpace(code)
	if trimmedCode == "" {
		return onboardingapp.InvalidInputError{Field: "code", Reason: "must not be empty"}
	}
	runtimeClient, phone, phoneCodeHash, err := g.runtimeClientLocked(authOperationVerifyCode)
	if err != nil {
		return err
	}
	runErr := runtimeClient.Run(ctx, func(runCtx context.Context, authClient authGatewayClient) error {
		_, signInErr := authClient.SignIn(runCtx, phone, trimmedCode, phoneCodeHash)
		return signInErr
	})
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
		return &onboardingapp.OperationError{
			Kind:      onboardingapp.ErrorKindInternal,
			Operation: authOperationVerifyPassword,
			Advice:    adviceForOnboardingErrorKind(onboardingapp.ErrorKindInternal),
			Err:       errors.New("telegram auth gateway is nil"),
		}
	}
	trimmedPassword := strings.TrimSpace(password)
	if trimmedPassword == "" {
		return onboardingapp.InvalidInputError{Field: "password", Reason: "must not be empty"}
	}
	runtimeClient, _, _, err := g.runtimeClientLocked(authOperationVerifyPassword)
	if err != nil {
		return err
	}
	runErr := runtimeClient.Run(ctx, func(runCtx context.Context, authClient authGatewayClient) error {
		_, passwordErr := authClient.Password(runCtx, trimmedPassword)
		return passwordErr
	})
	if runErr != nil {
		return mapTelegramAuthError(authOperationVerifyPassword, runErr)
	}
	return nil
}

func (g *LiveAuthGateway) runtimeClientLocked(operation string) (authGatewayRuntimeClient, string, string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.authorizedApp <= 0 || strings.TrimSpace(g.authorizedKey) == "" {
		return nil, "", "", &onboardingapp.OperationError{
			Kind:      onboardingapp.ErrorKindInternal,
			Operation: operation,
			Advice:    "start authentication before verifying code/password",
			Err:       errors.New("telegram auth context is not initialized"),
		}
	}
	runtimeClient := g.newRuntimeClient(g.authorizedApp, g.authorizedKey, g.sessionPath, g.proxyAddr)
	if runtimeClient == nil {
		return nil, "", "", &onboardingapp.OperationError{
			Kind:      onboardingapp.ErrorKindInternal,
			Operation: operation,
			Advice:    adviceForOnboardingErrorKind(onboardingapp.ErrorKindInternal),
			Err:       errors.New("telegram auth runtime client is nil"),
		}
	}
	if operation == authOperationVerifyCode && strings.TrimSpace(g.phoneCodeHash) == "" {
		return nil, "", "", &onboardingapp.OperationError{
			Kind:      onboardingapp.ErrorKindInternal,
			Operation: operation,
			Advice:    "start authentication before submitting verification code",
			Err:       errors.New("missing phone code hash from start operation"),
		}
	}
	return runtimeClient, g.phone, g.phoneCodeHash, nil
}

func parseAPIID(apiID string) (int, error) {
	trimmedAPIID := strings.TrimSpace(apiID)
	if trimmedAPIID == "" {
		return 0, onboardingapp.InvalidInputError{Field: "api_id", Reason: "must not be empty"}
	}
	parsedAPIID, err := strconv.Atoi(trimmedAPIID)
	if err != nil || parsedAPIID <= 0 {
		return 0, onboardingapp.InvalidInputError{Field: "api_id", Reason: "must be a positive integer"}
	}
	return parsedAPIID, nil
}

func extractPhoneCodeHash(sentCode tg.AuthSentCodeClass) (string, error) {
	if sentCode == nil {
		return "", errors.New("telegram send code returned nil response")
	}
	switch typed := sentCode.(type) {
	default:
		return "", fmt.Errorf("unsupported telegram send code response type %T", sentCode)
	case *tg.AuthSentCode:
		codeHash := strings.TrimSpace(typed.GetPhoneCodeHash())
		if codeHash == "" {
			return "", errors.New("telegram send code returned empty phone code hash")
		}
		return codeHash, nil
	case *tg.AuthSentCodePaymentRequired:
		codeHash := strings.TrimSpace(typed.GetPhoneCodeHash())
		if codeHash == "" {
			return "", errors.New("telegram send code payment-required response returned empty phone code hash")
		}
		return codeHash, nil
	}
}

func mapTelegramAuthError(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &onboardingapp.OperationError{
			Kind:      onboardingapp.ErrorKindTimeout,
			Operation: operation,
			Advice:    adviceForOnboardingErrorKind(onboardingapp.ErrorKindTimeout),
			Err:       err,
		}
	}
	if retryAfter, ok := tgerr.AsFloodWait(err); ok {
		return &onboardingapp.OperationError{
			Kind:       onboardingapp.ErrorKindRateLimit,
			Operation:  operation,
			RetryAfter: retryAfter,
			Advice:     adviceForOnboardingErrorKind(onboardingapp.ErrorKindRateLimit),
			Err:        err,
		}
	}
	if isTelegramInvalidTargetError(err) {
		return &onboardingapp.OperationError{
			Kind:      onboardingapp.ErrorKindInvalidTarget,
			Operation: operation,
			Advice:    adviceForOnboardingErrorKind(onboardingapp.ErrorKindInvalidTarget),
			Err:       err,
		}
	}
	if isTelegramAuthOperationError(err) {
		return &onboardingapp.OperationError{
			Kind:      onboardingapp.ErrorKindAuth,
			Operation: operation,
			Advice:    adviceForOnboardingErrorKind(onboardingapp.ErrorKindAuth),
			Err:       err,
		}
	}
	if isNetworkError(err) {
		return &onboardingapp.OperationError{
			Kind:      onboardingapp.ErrorKindNetwork,
			Operation: operation,
			Advice:    adviceForOnboardingErrorKind(onboardingapp.ErrorKindNetwork),
			Err:       err,
		}
	}
	return &onboardingapp.OperationError{
		Kind:      onboardingapp.ErrorKindInternal,
		Operation: operation,
		Advice:    adviceForOnboardingErrorKind(onboardingapp.ErrorKindInternal),
		Err:       err,
	}
}

func isTelegramAuthOperationError(err error) bool {
	if err == nil {
		return false
	}
	if tgauth.IsUnauthorized(err) || tgerr.IsCode(err, 401) {
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
	if strings.Contains(upperType, "PHONE_CODE") && (strings.Contains(upperType, "INVALID") || strings.Contains(upperType, "EXPIRED")) {
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
	if rpcErr.IsOneOf("USERNAME_INVALID", "CHANNEL_INVALID", "CHAT_ID_INVALID", "PEER_ID_INVALID", "USER_ID_INVALID") {
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

func (a authGatewayRuntimeClientAdapter) Run(ctx context.Context, callback func(context.Context, authGatewayClient) error) error {
	if a.client == nil {
		return errors.New("gotd auth client must not be nil")
	}
	return runTelegramClient(a.client, ctx, func(runCtx context.Context) error {
		return callback(runCtx, a.client.Auth())
	})
}

func defaultAuthGatewayRuntimeClientFactory(appID int, appHash, sessionPath, proxyAddr string) authGatewayRuntimeClient {
	return authGatewayRuntimeClientAdapter{client: gotdtelegram.NewClient(appID, appHash, telegramClientOptions(sessionPath, proxyAddr))}
}
