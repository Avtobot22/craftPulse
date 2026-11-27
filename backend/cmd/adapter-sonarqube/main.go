// Command adapter-sonarqube serves the adapter contract for SonarQube.
package main

import (
	"github.com/Avtobot22/craftPulse/backend/internal/adapter/sonarqube"
	"github.com/Avtobot22/craftPulse/backend/internal/app"
	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

func main() {
	app.AdapterMain("adapter-sonarqube", func(client *adaptersdk.SourceClient) (adaptersdk.Source, error) {
		return sonarqube.New(client), nil
	})
}
