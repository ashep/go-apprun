package apprun

import (
	"context"
	"errors"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ashep/go-cfgloader"
	"github.com/rs/zerolog"
)

type Config[CT any] struct {
	AppName   string
	AppVer    string
	LogLevel  zerolog.Level
	LogWriter io.Writer

	App CT
}

type app interface {
	Run(context.Context) error
}

type factory[AT app, CT any] func(cfg Config[CT]) (AT, error)

func Run[AT app, CT any](f factory[AT, CT], appCfg CT, appName, appVer string, lw io.Writer) int {
	time.Local = time.UTC
	ll := zerolog.InfoLevel

	dbg := os.Getenv("APP_DEBUG")
	if dbg == "true" || dbg == "1" {
		ll = zerolog.DebugLevel
	}

	if lw == nil {
		if isTerminal() {
			lw = zerolog.ConsoleWriter{Out: os.Stderr}
		} else {
			lw = os.Stderr
		}
	}

	if appName == "" {
		appName = os.Getenv("APP_NAME")
	}

	if appVer == "" {
		appVer = os.Getenv("APP_VERSION")
	}

	// Bootstrap logger, use only in this func
	bl := zerolog.New(lw).Level(ll).With().Str("app", appName).Str("app_v", appVer).Logger()

	for _, base := range []string{"config", appName} {
		for _, ext := range []string{".yaml", ".json"} {
			cfgPath := base + ext
			err := cfgloader.LoadFromPath(cfgPath, &appCfg, nil)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				bl.Error().Err(err).Str("path", cfgPath).Msg("config file load failed")
				return 1
			} else if err == nil {
				bl.Debug().Str("path", cfgPath).Msg("config file loaded")
			}
		}
	}

	if cfgPath := os.Getenv("APP_CONFIG_PATH"); cfgPath != "" {
		if err := cfgloader.LoadFromPath(cfgPath, &appCfg, nil); err != nil {
			bl.Error().Err(err).Str("path", cfgPath).Msg("config file load failed")
			return 1
		}

		bl.Debug().Str("path", cfgPath).Msg("config file loaded")
	}

	if err := cfgloader.LoadFromEnv("APP", &appCfg); err != nil {
		bl.Error().Err(err).Msg("load config from env vars failed")
		return 1
	}

	ctx, ctxC := context.WithCancel(context.Background())
	defer ctxC()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		s := <-sig
		bl.Info().Str("signal", s.String()).Msg("signal received")
		ctxC()
	}()

	cfg := Config[CT]{
		AppName:   appName,
		AppVer:    appVer,
		LogLevel:  ll,
		LogWriter: lw,
		App:       appCfg,
	}

	a, err := f(cfg)
	if err != nil {
		bl.Error().Err(err).Msg("app init failed")
		return 1
	}

	if err := a.Run(ctx); err != nil {
		bl.Error().Err(err).Msg("app run failed")
		return 1
	}

	return 0
}

func isTerminal() bool {
	if o, _ := os.Stdout.Stat(); (o.Mode() & os.ModeCharDevice) == os.ModeCharDevice {
		return true
	}

	return false
}
