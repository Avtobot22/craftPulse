package main

import (
	"os"

	"github.com/caarlos0/env/v11"

	"github.com/Avtobot22/craftPulse/backend/config"
	"github.com/Avtobot22/craftPulse/backend/internal/app"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

const _service = "worker"

func main() {
	boot := logger.Default(_service)
	cfg, err := config.Load(env.ToMap(os.Environ()))
	if err != nil {
		boot.Fatal().Err(err).Msg("config")
	}
	log, err := logger.New(os.Stdout, cfg.LogLevel, _service, cfg.Secrets())
	if err != nil {
		boot.Fatal().Err(err).Msg("logger")
	}
	if err := app.RunWorker(cfg, log); err != nil {
		log.Fatal().Err(err).Msg("worker stopped with error")
	}
}
