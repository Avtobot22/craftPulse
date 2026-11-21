// Command adapter-gitlab serves the adapter contract for GitLab self-managed.
package main

import (
	"github.com/Avtobot22/craftPulse/backend/internal/adapter/gitlab"
	"github.com/Avtobot22/craftPulse/backend/internal/app"
	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

func main() {
	app.AdapterMain("adapter-gitlab", func(client *adaptersdk.SourceClient) (adaptersdk.Source, error) {
		return gitlab.New(client), nil
	})
}
