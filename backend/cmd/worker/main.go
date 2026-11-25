package main

import (
	"errors"
	"os"

	"github.com/caarlos0/env/v11"

	"github.com/Avtobot22/craftPulse/backend/config"
	"github.com/Avtobot22/craftPulse/backend/internal/app"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

const _service = "worker"

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		if !health.CheckLocalLiveness(os.Getenv("HTTP_ADDR")) {
			os.Exit(1)
		}
		return
	}
	boot := logger.Default(_service)
	environ := env.ToMap(os.Environ())
	cfg, err := config.Load(environ)
	adapters, adaptersErr := config.LoadAdapters(environ)
	checks, checksErr := config.LoadChecks(environ)
	if err := errors.Join(err, adaptersErr, checksErr); err != nil {
		boot.Fatal().Err(err).Msg("config")
	}
	log, err := logger.New(os.Stdout, cfg.LogLevel, _service, append(cfg.Secrets(), adapters.Secrets()...))
	if err != nil {
		boot.Fatal().Err(err).Msg("logger")
	}
	if err := app.RunWorker(cfg, adapters, checks, log); err != nil {
		log.Fatal().Err(err).Msg("worker stopped with error")
	}
}
