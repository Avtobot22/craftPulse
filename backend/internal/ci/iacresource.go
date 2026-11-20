package ci

import (
	"bufio"
	"bytes"
	"os"
	"strings"
)

// configResource names the resource of a misconfiguration that Trivy reports without one: the kind and name of
// the Kubernetes object whose document holds the line, or the build stage of a Dockerfile. A finding about the
// whole file, or a file that cannot be read, gets the file itself.
func configResource(name, kind string, line int) string {
	data, err := os.ReadFile(name) //nolint:gosec // the file is a target of the report inside the checkout
	if err != nil || line <= 0 {
		return "file"
	}
	var resource string
	switch kind {
	case "kubernetes":
		resource = kubernetesObject(data, line)
	case "dockerfile":
		resource = dockerfileStage(data, line)
	}
	if resource == "" {
		return "file"
	}
	return resource
}

// kubernetesObject is "<kind>/<name>" of the YAML document that holds the line: its top-level kind and the
// name under its top-level metadata.
func kubernetesObject(data []byte, line int) string {
	sc := bufio.NewScanner(bytes.NewReader(data))
	var kind, name string
	inMetadata := false
	for no := 1; sc.Scan(); no++ {
		text := strings.TrimRight(sc.Text(), " \t\r")
		if strings.HasPrefix(text, "---") {
			if no > line {
				break
			}
			kind, name, inMetadata = "", "", false
			continue
		}
		top := text != "" && text[0] != ' ' && text[0] != '#'
		switch {
		case top && strings.HasPrefix(text, "kind:"):
			kind = yamlScalar(strings.TrimPrefix(text, "kind:"))
		case top:
			inMetadata = strings.HasPrefix(text, "metadata:")
		case inMetadata && name == "" && strings.HasPrefix(strings.TrimSpace(text), "name:"):
			name = yamlScalar(strings.TrimPrefix(strings.TrimSpace(text), "name:"))
		}
	}
	if kind == "" {
		return ""
	}
	return kind + "/" + name
}

func yamlScalar(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), " #")
	return strings.Trim(s, `"'`)
}

// dockerfileStage is "stage <alias or image>" of the last FROM at or before the line.
func dockerfileStage(data []byte, line int) string {
	sc := bufio.NewScanner(bytes.NewReader(data))
	stage := ""
	for no := 1; sc.Scan() && no <= line; no++ {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 || !strings.EqualFold(fields[0], "FROM") {
			continue
		}
		args := fields[1:]
		for len(args) > 1 && strings.HasPrefix(args[0], "--") {
			args = args[1:]
		}
		stage = "stage " + args[0]
		if len(args) == 3 && strings.EqualFold(args[1], "AS") {
			stage = "stage " + args[2]
		}
	}
	return stage
}
