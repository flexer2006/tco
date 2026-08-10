package telegram

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gotd/td/tg"

	"github.com/flexer2006/tco/internal/domain"
	"github.com/flexer2006/tco/internal/ports"
)

const defaultLiveHistoryPageSize = 100

type (
	LiveSourceOption func(*LiveSource)
	LiveSource       struct {
		appHash, sessionPath, proxyAddr string
		newRuntimeClient                liveRuntimeClientFactory
		pageSize, appID, maxMessages    int
		includeAllMessages              bool
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

func WithLiveSourceMaxMessages(maxMessages int) LiveSourceOption {
	return func(source *LiveSource) {
		if source == nil {
			return
		}

		source.maxMessages = maxMessages
	}
}

func WithLiveSourceIncludeAllMessages(includeAll bool) LiveSourceOption {
	return func(source *LiveSource) {
		if source == nil {
			return
		}

		source.includeAllMessages = includeAll
	}
}

func NewLiveSource(
	appID int,
	appHash, sessionPath string,
	options ...LiveSourceOption,
) (*LiveSource, error) {
	if appID <= 0 {
		return nil, ErrAppIDNotPositive
	}

	if strings.TrimSpace(appHash) == "" {
		return nil, ErrAppHashEmpty
	}

	if strings.TrimSpace(sessionPath) == "" {
		return nil, ErrSessionPathEmpty
	}

	err := os.MkdirAll(filepath.Dir(sessionPath), sessionDirMode)
	if err != nil {
		return nil, fmt.Errorf("create telegram session directory: %w", err)
	}

	source := new(LiveSource{
		appID:            appID,
		appHash:          strings.TrimSpace(appHash),
		sessionPath:      strings.TrimSpace(sessionPath),
		newRuntimeClient: defaultLiveRuntimeClientFactory,
		pageSize:         defaultLiveHistoryPageSize,
		maxMessages:      defaultHistoryMaxMessages,
	})

	for _, option := range options {
		if option != nil {
			option(source)
		}
	}

	if source.newRuntimeClient == nil {
		return nil, ErrLiveRuntimeClientFactoryNil
	}

	if source.pageSize <= 0 {
		return nil, ErrLiveHistoryPageSizeMustBePositive
	}

	if source.maxMessages <= 0 {
		return nil, ErrLiveHistoryMaxMessagesNotPositive
	}

	return source, nil
}

func (s *LiveSource) FetchMessages(
	ctx context.Context,
	req ports.MessageFetchRequest,
) ([]domain.RawCanonicalMessage, error) {
	if s == nil {
		return nil, ErrLiveSourceNil
	}

	if ctx == nil {
		return nil, ErrContextNil
	}

	trimmedSourceChat := strings.TrimSpace(req.SourceChat)
	if trimmedSourceChat == "" {
		return nil, ErrSourceChatEmpty
	}

	maxMessages := s.maxMessages
	if req.MaxMessages > 0 {
		maxMessages = req.MaxMessages
	}

	target, err := parseLiveTarget(trimmedSourceChat)
	if err != nil {
		return nil, fmt.Errorf("parse telegram live source chat %q: %w", trimmedSourceChat, err)
	}

	runtimeClient := s.newRuntimeClient(s.appID, s.appHash, s.sessionPath, s.proxyAddr)

	var rawMessages []tg.MessageClass

	runErr := runtimeClient.Run(
		ctx,
		func(runCtx context.Context, api liveAPI, authClient liveAuthClient) error {
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

			history, err := fetchLiveHistory(
				runCtx,
				api,
				peer,
				s.pageSize,
				maxMessages,
				req.MinExclusiveMessageID,
			)
			if err != nil {
				return wrapLiveError("fetch history", err)
			}

			rawMessages = history

			return nil
		},
	)
	if runErr != nil {
		return nil, wrapLiveError("run live telegram session", runErr)
	}

	canonical, err := mapLiveMessages(trimmedSourceChat, rawMessages, s.includeAllMessages)
	if err != nil {
		return nil, err
	}

	return canonical, nil
}
