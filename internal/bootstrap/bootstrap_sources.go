package bootstrap

import (
	"fmt"
	"strconv"
	"strings"
	"github.com/flexer2006/tco/internal/adapters"
	"github.com/flexer2006/tco/internal/application"
	"github.com/flexer2006/tco/internal/config"
)

func buildTelegramSourceWithDeps(cfg config.Config, deps deps) (application.TelegramSource, error) {
	deps = deps.withDefaults()
	if cfg.TelegramSourceMode != telegramSourceModeLive {
		return nil, fmt.Errorf("TELEGRAM_SOURCE_MODE must be %s for production runtime, got %q", telegramSourceModeLive, cfg.TelegramSourceMode)
	}
	appID, err := strconv.Atoi(strings.TrimSpace(cfg.TelegramAPIID))
	if err != nil {
		return nil, fmt.Errorf("parse TELEGRAM_API_ID: %w", err)
	}
	options := make([]adapters.LiveSourceOption, 0, 1)
	if strings.TrimSpace(cfg.TelegramProxyAddr) != "" {
		options = append(options, adapters.WithLiveSourceProxyAddr(cfg.TelegramProxyAddr))
	}
	source, err := deps.NewLiveSource(appID, cfg.TelegramAPIHash, cfg.TelegramSessionPath, options...)
	if err != nil {
		return nil, fmt.Errorf("create live telegram source: %w", err)
	}
	return source, nil
}
