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

const _service = "api"

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		if !health.CheckLocalLiveness(os.Getenv("HTTP_ADDR")) {
			os.Exit(1)
		}
		return
	}
	boot := logger.Default(_service)
	command := ""
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	environ := env.ToMap(os.Environ())
	cfg, err := config.Load(environ)
	var (
		in       config.Install
		site     config.Site
		adapters config.Adapters
		checks   config.Checks
	)
	switch command {
	case "install":
		var inErr, adaptersErr error
		in, inErr = config.LoadInstall(environ)
		adapters, adaptersErr = config.LoadAdapters(environ)
		err = errors.Join(err, inErr, adaptersErr)
	case "migrate":
	default:
		var siteErr, adaptersErr, checksErr error
		site, siteErr = config.LoadSite(environ)
		adapters, adaptersErr = config.LoadAdapters(environ)
		checks, checksErr = config.LoadChecks(environ)
		err = errors.Join(err, siteErr, adaptersErr, checksErr)
	}
	if err != nil {
		boot.Fatal().Err(err).Msg("config")
	}
	log, err := logger.New(os.Stdout, cfg.LogLevel, _service, append(append(cfg.Secrets(), in.Secrets()...), adapters.Secrets()...))
	if err != nil {
		boot.Fatal().Err(err).Msg("logger")
	}

	switch command {
	case "migrate":
		err = app.RunMigrate(cfg, log)
	case "install":
		err = app.RunInstall(cfg, in, adapters, log)
	default:
		err = app.RunAPI(cfg, site, adapters, checks, log)
	}
	if err != nil {
		log.Fatal().Err(err).Msg("api stopped with error")
	}
}
