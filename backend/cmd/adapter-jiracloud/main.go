// Command adapter-jiracloud serves the adapter contract for Jira Cloud.
package main

import (
	"github.com/Avtobot22/craftPulse/backend/internal/adapter/jiracloud"
	"github.com/Avtobot22/craftPulse/backend/internal/app"
	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

func main() {
	app.AdapterMain("adapter-jiracloud", func(client *adaptersdk.SourceClient) (adaptersdk.Source, error) {
		return jiracloud.New(client), nil
	})
}
