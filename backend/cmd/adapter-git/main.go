// Command adapter-git serves documents and refs of Git repositories by ssh or https (TZ 5.5).
package main

import (
	"os"

	"github.com/caarlos0/env/v11"

	"github.com/Avtobot22/craftPulse/backend/config"
	gitadapter "github.com/Avtobot22/craftPulse/backend/internal/adapter/git"
	"github.com/Avtobot22/craftPulse/backend/internal/app"
	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

func main() {
	app.AdapterMain("adapter-git", func(client *adaptersdk.SourceClient) (adaptersdk.Source, error) {
		cfg, err := config.LoadAdapterGit(env.ToMap(os.Environ()))
		if err != nil {
			return nil, err
		}
		return gitadapter.New(gitadapter.Config{
			CacheDir: cfg.CacheDir, CacheBytes: cfg.CacheBytes, CloneBytes: cfg.CloneBytes, CloneTimeout: cfg.CloneTimeout,
			CAFile: cfg.SourceCAFile,
		}, client)
	})
}
