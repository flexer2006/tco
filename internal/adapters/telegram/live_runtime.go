package telegram

import (
	"context"

	gotdtelegram "github.com/gotd/td/telegram"
	tgauth "github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
)

var runTelegramClient = (*gotdtelegram.Client).Run

type (
	liveAuthClient interface {
		Status(ctx context.Context) (*tgauth.Status, error)
	}
	liveAPI interface {
		ContactsResolveUsername(
			ctx context.Context,
			request *tg.ContactsResolveUsernameRequest,
		) (*tg.ContactsResolvedPeer, error)
		MessagesGetDialogs(
			ctx context.Context,
			request *tg.MessagesGetDialogsRequest,
		) (tg.MessagesDialogsClass, error)
		MessagesGetHistory(
			ctx context.Context,
			request *tg.MessagesGetHistoryRequest,
		) (tg.MessagesMessagesClass, error)
	}
	liveRuntimeClient interface {
		Run(
			ctx context.Context,
			callback func(ctx context.Context, api liveAPI, authClient liveAuthClient) error,
		) error
	}
	liveRuntimeClientFactory func(appID int, appHash, sessionPath, proxyAddr string) liveRuntimeClient
	liveRuntimeClientAdapter struct {
		client *gotdtelegram.Client
	}
)

func (a liveRuntimeClientAdapter) Run(
	ctx context.Context,
	callback func(ctx context.Context, api liveAPI, authClient liveAuthClient) error,
) error {
	if a.client == nil {
		return ErrGotdClientNil
	}

	return runTelegramClient(a.client, ctx, func(runCtx context.Context) error {
		return callback(runCtx, a.client.API(), a.client.Auth())
	})
}

func defaultLiveRuntimeClientFactory(
	appID int,
	appHash, sessionPath, proxyAddr string,
) liveRuntimeClient {
	options, err := telegramClientOptions(sessionPath, proxyAddr)
	if err != nil {
		return failedRuntimeClient{err: err}
	}

	return liveRuntimeClientAdapter{client: gotdtelegram.NewClient(appID, appHash, options)}
}
