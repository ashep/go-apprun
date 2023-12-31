package apprun

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/ashep/go-cfgloader"
)

var (
	appName = "app"
	appVer  = "unknown"
)

type App interface {
	Run(ctx context.Context, args []string) error
}

type factory[AT App, CT any] func(cfg CT, l zerolog.Logger) AT

func Run[AT App, CT any](f factory[AT, CT], cfg CT) {
	time.Local = time.UTC

	ll := zerolog.InfoLevel
	dbg := os.Getenv("APP_DEBUG")
	if dbg == "true" || dbg == "1" {
		ll = zerolog.DebugLevel
	}

	l := log.Logger.Level(ll).With().Str("app", appName).Str("app_v", appVer).Logger()
	if o, _ := os.Stdout.Stat(); (o.Mode() & os.ModeCharDevice) == os.ModeCharDevice { // Terminal
		l = l.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	if cfgPath := os.Getenv("APP_CONFIG_PATH"); cfgPath != "" {
		if err := cfgloader.LoadFromPath(cfgPath, &cfg, nil); err != nil {
			l.Error().Err(err).Msg("load config from file failed")
			os.Exit(1)
		}
		l.Debug().Str("path", cfgPath).Msg("config loaded from file")
	}

	appEnvCfgName := strings.ReplaceAll(appName, "-", "_")
	appEnvCfgName = strings.ReplaceAll(appEnvCfgName, ".", "_")
	if err := cfgloader.LoadFromEnv(appEnvCfgName, &cfg); err != nil {
		l.Error().Err(err).Msg("load config from env failed")
		os.Exit(1)
	}

	ctx, ctxC := context.WithCancel(context.Background())
	defer ctxC()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		s := <-sig
		l.Info().Str("signal", s.String()).Msg("signal received")
		ctxC()
	}()

	if err := f(cfg, l).Run(ctx, os.Args); err != nil {
		l.Error().Err(err).Msg("app run failed")
		os.Exit(1)
	}
}
