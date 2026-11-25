package change

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

type actionsStep struct {
	Uses string            `yaml:"uses,omitempty"`
	With map[string]any    `yaml:"with,omitempty"`
	Env  map[string]string `yaml:"env,omitempty"`
	Run  string            `yaml:"run,omitempty"`
}

type actionsJob struct {
	Name      string `yaml:"name"`
	If        string `yaml:"if,omitempty"`
	RunsOn    string `yaml:"runs-on"` //nolint:tagliatelle // GitHub Actions requires this key.
	Container struct {
		Image   string `yaml:"image"`
		Options string `yaml:"options"`
	} `yaml:"container"`
	Steps []actionsStep `yaml:"steps"`
}

func actionsSnippet(jobs map[string]gitlabJob, variables []entity.CIVariable) ([]byte, error) {
	out := struct {
		Name        string                `yaml:"name"`
		On          []string              `yaml:"on"`
		Permissions map[string]string     `yaml:"permissions"`
		Jobs        map[string]actionsJob `yaml:"jobs"`
	}{
		Name: "craftPulse (advisory)", On: []string{"push", "pull_request", "workflow_dispatch"},
		Permissions: map[string]string{"actions": "read", "contents": "read"},
		Jobs:        make(map[string]actionsJob, len(jobs)),
	}
	if len(jobs) == 0 {
		return []byte("# No craftPulse checks are configured.\n"), nil
	}
	uploadSecretRef := "${{ secrets." + _varToken + " }}"
	for name, job := range jobs {
		j := actionsJob{
			Name: name, RunsOn: "ubuntu-latest",
			If: "${{ github.actor != 'dependabot[bot]' && (github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository) }}",
		}
		j.Container.Image = job.Image
		// The runner mounts a checkout owned by its host UID.
		j.Container.Options = "--user 0"
		scanEnv := map[string]string{
			_varToken: uploadSecretRef, _varURL: "${{ vars.CRAFTPULSE_URL }}",
		}
		if name == _importJob {
			for _, variable := range variables {
				if strings.HasPrefix(variable.Name, _reportVar) {
					scanEnv[variable.Name] = "${{ vars." + variable.Name + " }}"
				}
			}
		}
		j.Steps = []actionsStep{
			{Run: `chown "$(id -u):$(id -g)" "$GITHUB_WORKSPACE"`},
			{Uses: "actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1", With: map[string]any{"persist-credentials": false, "fetch-depth": 0}},
			{Env: scanEnv, Run: strings.Join(job.Script[:len(job.Script)-1], "\n")},
			{Env: map[string]string{ //nolint:gosec // G101: GitHub Actions resolves this token reference at runtime.
				_varToken: uploadSecretRef, _varURL: "${{ vars.CRAFTPULSE_URL }}",
				"CRAFTPULSE_ACTIONS_TOKEN": "${{ github.token }}",
			}, Run: "craftpulse-ci upload"},
		}
		out.Jobs[name] = j
	}
	return yaml.Marshal(out)
}

// groovyString keeps Groovy interpolation out of shell source and image references.
func groovyString(value string) string {
	value = strings.NewReplacer("\\", "\\\\", "'", "\\'", "\n", "\\n", "\r", "\\r").Replace(value)
	return "'" + value + "'"
}

func jenkinsSnippet(jobs map[string]gitlabJob) string {
	if len(jobs) == 0 {
		return "// No craftPulse checks are configured.\n"
	}
	names := slices.Sorted(maps.Keys(jobs))
	var b strings.Builder
	b.WriteString(`// Add to stages in a Declarative Pipeline with a Docker-enabled agent and SCM configured.
// Set CRAFTPULSE_URL in the job and create a Secret text credential named CRAFTPULSE_UPLOAD_TOKEN.
stage('craftPulse (advisory)') {
    steps {
        script {
            def scmVars = checkout scm
            def base = pwd(tmp: true) + '/craftpulse-' + env.BUILD_NUMBER
            def attemptsDir = base + '/attempts'
            def manifest = pwd() + '/execution-manifest.json'
            withEnv(["GIT_COMMIT=${scmVars.GIT_COMMIT}", "GIT_BRANCH=${scmVars.GIT_BRANCH}",
                     "GIT_PREVIOUS_COMMIT=${scmVars.GIT_PREVIOUS_COMMIT ?: ''}",
                     "CP_ATTEMPTS_DIR=${attemptsDir}", "CP_MANIFEST=${manifest}"]) {
                withCredentials([string(credentialsId: 'CRAFTPULSE_UPLOAD_TOKEN', variable: 'CRAFTPULSE_UPLOAD_TOKEN')]) {
                    def scans = [
`)
	for _, name := range names {
		job := jobs[name]
		script := `#!/bin/sh
set -eu
started=$(date -u +%Y-%m-%dT%H:%M:%SZ)
status=0
sh -ec ` + shellString(strings.Join(job.Script[:len(job.Script)-1], "\n")) + ` || status=$?
outcome=success
if [ "$status" -ne 0 ]; then outcome=failure; fi
craftpulse-ci jenkins record --attempts-dir "$CP_ATTEMPTS_DIR" --stage "craftPulse (advisory)" --step "$CP_STEP" --attempt "$CP_ATTEMPT" --started-at "$started" --outcome "$outcome" > "$CRAFTPULSE_WORK_DIR/attempt-id"
exit "$status"
`
		fmt.Fprintf(&b, "                        [name: %s, image: %s, script: %s],\n", groovyString(name), groovyString(job.Image), groovyString(script))
	}
	fmt.Fprintf(&b, `                    ]
                    try {
                        scans.each { scan ->
                            def attempt = 0
                            docker.image(scan.image).inside {
                                retry(2) {
                                    attempt++
                                    scan.work = base + '/' + scan.name + '-' + attempt
                                    withEnv(["CRAFTPULSE_WORK_DIR=${scan.work}", "CP_ATTEMPT=${attempt}", "CP_STEP=${scan.name}"]) {
                                        sh 'mkdir -p "$CRAFTPULSE_WORK_DIR"'
                                        sh scan.script
                                    }
                                }
                            }
                        }
                    } finally {
                        docker.image(%s).inside {
                            sh 'craftpulse-ci jenkins finish --attempts-dir "$CP_ATTEMPTS_DIR" --output "$CP_MANIFEST"'
                        }
                        archiveArtifacts artifacts: 'execution-manifest.json'
                    }
                    scans.each { scan ->
                        docker.image(scan.image).inside {
                            withEnv(["CRAFTPULSE_WORK_DIR=${scan.work}"]) {
                                sh %s
                            }
                        }
                    }
                }
            }
        }
    }
}
`, groovyString(jobs[names[0]].Image), groovyString(`#!/bin/sh
set -eu
craftpulse-ci upload --execution-manifest "$CP_MANIFEST" --jenkins-attempt-id "$(cat "$CRAFTPULSE_WORK_DIR/attempt-id")"
`))
	return b.String()
}

func shellString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
