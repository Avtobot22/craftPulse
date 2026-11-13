// Command adapter-test serves recorded Jira Cloud answers with faults from TEST_ADAPTER_SCENARIO_FILE. It is
// for tests and the local stand only and is not part of a release image.
package main

import (
	"os"

	"github.com/Avtobot22/craftPulse/backend/internal/adapter/test"
	"github.com/Avtobot22/craftPulse/backend/internal/app"
	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

func main() {
	app.AdapterMain("adapter-test", func(*adaptersdk.SourceClient) (adaptersdk.Source, error) {
		return testadapter.NewFromFile(os.Getenv("TEST_ADAPTER_SCENARIO_FILE"))
	})
}
