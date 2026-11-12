package main

import (
	"errors"
	"os"

	"github.com/caarlos0/env/v11"

	"github.com/Avtobot22/craftPulse/backend/config"
	"github.com/Avtobot22/craftPulse/backend/internal/app"
	"github.com/Avtobot22/craftPulse/backend/pkg/logger"
)

const _service = "api"

func main() {
	boot := logger.Default(_service)
	command := ""
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	environ := env.ToMap(os.Environ())
	cfg, err := config.Load(environ)
	var (
		in   config.Install
		site config.Site
	)
	switch command {
	case "install":
		var inErr error
		in, inErr = config.LoadInstall(environ)
		err = errors.Join(err, inErr)
	case "migrate":
	default:
		var siteErr error
		site, siteErr = config.LoadSite(environ)
		err = errors.Join(err, siteErr)
	}
	if err != nil {
		boot.Fatal().Err(err).Msg("config")
	}
	log, err := logger.New(os.Stdout, cfg.LogLevel, _service, append(cfg.Secrets(), in.Secrets()...))
	if err != nil {
		boot.Fatal().Err(err).Msg("logger")
	}

	switch command {
	case "migrate":
		err = app.RunMigrate(cfg, log)
	case "install":
		err = app.RunInstall(cfg, in, log)
	default:
		err = app.RunAPI(cfg, site, log)
	}
	if err != nil {
		log.Fatal().Err(err).Msg("api stopped with error")
	}
}
