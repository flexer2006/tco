package bootstrap

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/flexer2006/tco/internal/adapters/telegram"
	"github.com/flexer2006/tco/internal/config"
	"github.com/flexer2006/tco/internal/ports"
)

func (c container) telegramSource(cfg config.Config) (ports.TelegramSource, error) {
	deps := c.withDefaults()

	if cfg.TelegramSourceMode != telegramSourceModeLive {
		return nil, fmt.Errorf(
			"%w, got %q",
			ErrTelegramSourceMode,
			cfg.TelegramSourceMode,
		)
	}

	appID, err := telegramAppID(cfg.TelegramAPIID)
	if err != nil {
		return nil, err
	}

	options := []telegram.LiveSourceOption{
		telegram.WithLiveSourceMaxMessages(cfg.HistoryMaxMessages),
		telegram.WithLiveSourceIncludeAllMessages(cfg.TelegramIncludeAllMessages),
	}
	if proxy := strings.TrimSpace(cfg.TelegramProxyAddr); proxy != "" {
		options = append(options, telegram.WithLiveSourceProxyAddr(proxy))
	}

	source, err := deps.NewLiveSource(
		appID,
		cfg.TelegramAPIHash,
		cfg.TelegramSessionPath,
		options...)
	if err != nil {
		return nil, fmt.Errorf("create live telegram source: %w", err)
	}

	return source, nil
}

func telegramAppID(raw string) (int, error) {
	appID, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("parse TELEGRAM_API_ID: %w", err)
	}

	return appID, nil
}
