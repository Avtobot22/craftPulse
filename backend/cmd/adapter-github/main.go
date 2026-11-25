// Command adapter-github serves the adapter contract for GitHub.com.
package main

import (
	"github.com/Avtobot22/craftPulse/backend/internal/adapter/github"
	"github.com/Avtobot22/craftPulse/backend/internal/app"
	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

func main() {
	app.AdapterMain("adapter-github", func(client *adaptersdk.SourceClient) (adaptersdk.Source, error) {
		return github.New(client), nil
	})
}
