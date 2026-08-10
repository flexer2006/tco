package telegram

import (
	"context"
	"fmt"
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

func telegramClientOptions(sessionPath, proxyAddr string) (gotdtelegram.Options, error) {
	options := gotdtelegram.Options{
		SessionStorage: new(session.FileStorage{Path: sessionPath}),
	}

	trimmedProxyAddr := strings.TrimSpace(proxyAddr)
	if trimmedProxyAddr == "" {
		return options, nil
	}

	_, _, err := net.SplitHostPort(trimmedProxyAddr)
	if err != nil {
		return gotdtelegram.Options{}, fmt.Errorf(
			"configure SOCKS5 proxy %q: must be host:port: %w",
			trimmedProxyAddr,
			err,
		)
	}

	dialer, err := proxy.SOCKS5("tcp", trimmedProxyAddr, nil, proxy.Direct)
	if err != nil {
		return gotdtelegram.Options{}, fmt.Errorf(
			"configure SOCKS5 proxy %q: %w",
			trimmedProxyAddr,
			err,
		)
	}

	options.Resolver = dcs.Plain(dcs.PlainOptions{
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if ctxDialer, ok := dialer.(contextDialer); ok {
				return ctxDialer.DialContext(ctx, network, addr)
			}

			return dialer.Dial(network, addr)
		},
	})

	return options, nil
}

type failedRuntimeClient struct {
	err error
}

func (c failedRuntimeClient) Run(
	context.Context,
	func(context.Context, liveAPI, liveAuthClient) error,
) error {
	return c.err
}

type failedAuthRuntimeClient struct {
	err error
}

func (c failedAuthRuntimeClient) Run(
	context.Context,
	func(context.Context, authGatewayClient) error,
) error {
	return c.err
}
