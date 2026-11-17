package dataset

import (
	"fmt"
	"strings"
)

func pipelineDefinition(run *CIRun, s ciScenario) (string, string) {
	switch run.Provider {
	case "gitlab":
		return ".gitlab-ci.yml", gitlabDefinition(run, s)
	case "github":
		return ".github/workflows/demo.yml", githubDefinition(run, s)
	default:
		return "Jenkinsfile", jenkinsDefinition(run)
	}
}

func gitlabDefinition(run *CIRun, s ciScenario) string {
	var b strings.Builder
	b.WriteString("workflow:\n  rules:\n    - if: '$CI_PIPELINE_SOURCE == \"api\" || $CI_PIPELINE_SOURCE == \"web\" || $CI_PIPELINE_SOURCE == \"trigger\"'\n    - when: never\ndefault:\n  image: golang:1.27.4-alpine\nstages: [test, verify]\n")
	for _, job := range run.Jobs {
		if job.Name == "test" && job.Axes["mode"] != "fast" {
			continue
		}
		fmt.Fprintf(&b, "%s:\n  stage: %s\n  timeout: 1m\n  retry: 0\n  script:\n    - go run ./.demo-ci -job %s\n", job.Name, job.Stage, job.Name)
		if job.Name == "test" && s.matrix {
			b.WriteString("  parallel:\n    matrix:\n      - DEMO_MODE: [fast, full]\n")
		}
		if s.name == "manual" {
			b.WriteString("  when: manual\n  allow_failure: false\n")
		} else {
			fmt.Fprintf(&b, "  allow_failure: %t\n", job.AllowFailure)
		}
		if job.AllowFailure {
			b.WriteString("  variables:\n    DEMO_ALLOW_FAILURE: 'true'\n")
		}
		if len(job.Attempts) > 0 && len(job.Attempts[0].Reports) > 0 {
			b.WriteString("  artifacts:\n    when: always\n    paths: [reports/]\n    reports:\n")
			if s.junit {
				b.WriteString("      junit: reports/*/junit.xml\n")
			}
			if s.coverage {
				b.WriteString("      coverage_report:\n        coverage_format: cobertura\n        path: reports/*/coverage.xml\n")
			}
		}
	}
	return b.String()
}

func githubDefinition(run *CIRun, s ciScenario) string {
	var b strings.Builder
	b.WriteString("name: Demo CI\non:\n  workflow_dispatch:\npermissions:\n  contents: read\njobs:\n  test:\n    name: test\n    runs-on: ubuntu-24.04\n    timeout-minutes: 1\n")
	if s.matrix {
		b.WriteString("    strategy:\n      fail-fast: false\n      matrix:\n        mode: [fast, full]\n    env:\n      DEMO_MODE: ${{ matrix.mode }}\n")
	}
	b.WriteString("    steps:\n      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1\n      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e\n        with:\n          go-version: '1.27.4'\n          cache: false\n      - run: go run ./.demo-ci -job test\n")
	if s.junit || s.coverage {
		b.WriteString("      - uses: actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02\n        if: always()\n        with:\n          name: reports-${{ strategy.job-index }}-a${{ github.run_attempt }}\n          path: reports/\n          if-no-files-found: error\n")
	}
	return b.String()
}

func jenkinsDefinition(run *CIRun) string {
	var b strings.Builder
	b.WriteString("node('demo-go') {\n  deleteDir()\n  checkout scm\n  sh 'go build -o .demo-ci/fixture ./.demo-ci'\n  try {\n")
	for _, job := range run.Jobs {
		fmt.Fprintf(&b, "    stage('%s-%s') {\n      def attempt = 0\n      retry(%d) {\n        attempt++\n        withEnv([\"DEMO_ATTEMPT=${attempt}\", 'DEMO_MODE=%s']) {\n          sh '.demo-ci/fixture -job %s'\n        }\n      }\n    }\n", job.Stage, job.Axes["mode"], len(job.Attempts), job.Axes["mode"], job.Name)
	}
	b.WriteString("  } finally {\n    archiveArtifacts artifacts: 'reports/**', allowEmptyArchive: false\n")
	if run.ExecutionManifest {
		b.WriteString("    archiveArtifacts artifacts: 'execution-manifest.json', allowEmptyArchive: false\n")
	}
	b.WriteString("  }\n}\n")
	return b.String()
}
