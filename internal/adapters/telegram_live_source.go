package adapters

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"github.com/flexer2006/tco/internal/domain"

	"github.com/gotd/td/tg"
)

const defaultLiveHistoryPageSize = 100

type (
	LiveSourceOption func(*LiveSource)
	LiveSource       struct {
		appHash, sessionPath, proxyAddr string
		newRuntimeClient     liveRuntimeClientFactory
		pageSize, appID      int
	}
)

func WithLiveSourceProxyAddr(proxyAddr string) LiveSourceOption {
	return func(source *LiveSource) {
		if source == nil {
			return
		}
		source.proxyAddr = strings.TrimSpace(proxyAddr)
	}
}

func NewLiveSource(appID int, appHash, sessionPath string, options ...LiveSourceOption) (*LiveSource, error) {
	if appID <= 0 {
		return nil, errors.New("telegram app id must be positive")
	}
	if strings.TrimSpace(appHash) == "" {
		return nil, errors.New("telegram app hash must not be empty")
	}
	if strings.TrimSpace(sessionPath) == "" {
		return nil, errors.New("telegram session path must not be empty")
	}
	if err := os.MkdirAll(filepath.Dir(sessionPath), 0o700); err != nil {
		return nil, fmt.Errorf("create telegram session directory: %w", err)
	}
	source := &LiveSource{
		appID:            appID,
		appHash:          strings.TrimSpace(appHash),
		sessionPath:      strings.TrimSpace(sessionPath),
		newRuntimeClient: defaultLiveRuntimeClientFactory,
		pageSize:         defaultLiveHistoryPageSize,
	}
	for _, option := range options {
		if option != nil {
			option(source)
		}
	}
	if source.newRuntimeClient == nil {
		return nil, errors.New("telegram live runtime client factory must not be nil")
	}
	if source.pageSize <= 0 {
		return nil, errors.New("telegram live history page size must be positive")
	}
	return source, nil
}

func (s *LiveSource) FetchMessages(ctx context.Context, sourceChat string) ([]domain.RawCanonicalMessage, error) {
	if s == nil {
		return nil, errors.New("telegram live source must not be nil")
	}
	if ctx == nil {
		return nil, errors.New("context must not be nil")
	}
	trimmedSourceChat := strings.TrimSpace(sourceChat)
	if trimmedSourceChat == "" {
		return nil, errors.New("source chat must not be empty")
	}
	target, err := parseLiveTarget(trimmedSourceChat)
	if err != nil {
		return nil, fmt.Errorf("parse telegram live source chat %q: %w", trimmedSourceChat, err)
	}
	runtimeClient := s.newRuntimeClient(s.appID, s.appHash, s.sessionPath, s.proxyAddr)
	var rawMessages []tg.MessageClass
	runErr := runtimeClient.Run(ctx, func(runCtx context.Context, api liveAPI, authClient liveAuthClient) error {
		status, err := authClient.Status(runCtx)
		if err != nil {
			return wrapLiveError("check authorization", err)
		}
		if status == nil || !status.Authorized {
			return wrapLiveError("check authorization", errLiveSessionUnauthorized)
		}
		peer, err := resolveLiveInputPeer(runCtx, api, target)
		if err != nil {
			return wrapLiveError("resolve chat", err)
		}
		history, err := fetchLiveHistory(runCtx, api, peer, s.pageSize)
		if err != nil {
			return wrapLiveError("fetch history", err)
		}
		rawMessages = history
		return nil
	})
	if runErr != nil {
		return nil, wrapLiveError("run live telegram session", runErr)
	}
	canonical, err := mapLiveMessages(trimmedSourceChat, rawMessages)
	if err != nil {
		return nil, err
	}
	return canonical, nil
}
