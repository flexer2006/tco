package adapters

import (
	"context"
	"net"
	"strings"

	"github.com/gotd/td/session"
	gotdtelegram "github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	"golang.org/x/net/proxy"
)

type contextDialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

func telegramClientOptions(sessionPath, proxyAddr string) gotdtelegram.Options {
	options := gotdtelegram.Options{
		SessionStorage: &session.FileStorage{Path: sessionPath},
	}
	trimmedProxyAddr := strings.TrimSpace(proxyAddr)
	if trimmedProxyAddr == "" {
		return options
	}
	dialer, err := proxy.SOCKS5("tcp", trimmedProxyAddr, nil, proxy.Direct)
	if err != nil {
		return options
	}
	options.Resolver = dcs.Plain(dcs.PlainOptions{
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if ctxDialer, ok := dialer.(contextDialer); ok {
				return ctxDialer.DialContext(ctx, network, addr)
			}
			return dialer.Dial(network, addr)
		},
	})
	return options
}
