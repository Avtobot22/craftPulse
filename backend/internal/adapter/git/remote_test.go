package gitadapter

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

func TestNormalize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		give, want string
		wantErr    bool
	}{
		{give: "ssh://git@gitlab.example.org/group/app.git", want: "gitlab.example.org/group/app"},
		{give: "git@gitlab.example.org:group/app.git", want: "gitlab.example.org/group/app"},
		{give: "https://GitLab.Example.org/group/app/", want: "gitlab.example.org/group/app"},
		{give: "https://gitlab.example.org:443/group/app", want: "gitlab.example.org/group/app"},
		{give: "ssh://git@gitlab.example.org:22/group/app", want: "gitlab.example.org/group/app"},
		{give: "ssh://git@gitlab.example.org:2222/group/app.git", want: "gitlab.example.org:2222/group/app"},
		{give: "https://git.example.org:8443/a/b/c.git", want: "git.example.org:8443/a/b/c"},
		{give: "git.example.org:8443/a/b", want: "git.example.org:8443/a/b"},
		{give: "git.example.org/a/b", want: "git.example.org/a/b"},
		{give: "file:///srv/repo.git", wantErr: true},
		{give: "http://git.example.org/a", wantErr: true},
		{give: "ext::sh -c touch% /tmp/x", wantErr: true},
		{give: "https://git.example.org/-upload-pack=touch", wantErr: true},
		{give: "https://git.example.org/a/../b", wantErr: true},
		{give: "https://git.example.org/a b", wantErr: true},
		{give: "https://git.example.org/a?x=1", wantErr: true},
		{give: "https://git.example.org/", wantErr: true},
		{give: "https://-oProxyCommand=x/a", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.give, func(t *testing.T) {
			t.Parallel()
			got, err := Normalize(tt.give)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Normalize(%q) = %q, want an error", tt.give, got)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("Normalize(%q) = %q, %v; want %q", tt.give, got, err, tt.want)
			}
		})
	}
}

func TestRemoteOf(t *testing.T) {
	t.Parallel()
	srv, err := ParseServer("ssh://git.example.org:2222")
	if err != nil {
		t.Fatal(err)
	}
	rem, err := RemoteOf(srv, "git.example.org:2222/group/app")
	if err != nil {
		t.Fatal(err)
	}
	if got := rem.URL("git"); got != "ssh://git@git.example.org:2222/group/app" {
		t.Fatalf("URL %q", got)
	}
	if srv.Key() != "git.example.org:2222" {
		t.Fatalf("key %q", srv.Key())
	}
	for _, scope := range []string{
		"other.example.org:2222/group/app", "git.example.org/group/app", "git.example.org:2222/group/app.git",
		"ssh://git.example.org:2222/group/app", "git.example.org:2222/-x",
	} {
		if _, err := RemoteOf(srv, scope); err == nil {
			t.Errorf("scope %q of another server or not normalized is accepted", scope)
		}
	}
	https, _ := ParseServer("https://git.example.org")
	if rem, _ := RemoteOf(https, "git.example.org/a"); rem.URL("") != "https://git.example.org/a" {
		t.Fatalf("https URL %q", rem.URL(""))
	}
	for _, raw := range []string{"ssh://git@git.example.org", "https://git.example.org/path", "file:///x", "https://127.0.0.1:0"} {
		if _, err := ParseServer(raw); err == nil {
			t.Errorf("base_url %q is accepted", raw)
		}
	}
}

func TestParseCredentials(t *testing.T) {
	t.Parallel()
	c, err := parseCredentials(`{"token":"t0k"}`, "https")
	if err != nil || c.Username != "oauth2" {
		t.Fatalf("https defaults: %+v %v", c, err)
	}
	c, err = parseCredentials(`{"private_key":"k","known_hosts":"h"}`, "ssh")
	if err != nil || c.SSHUser != "git" {
		t.Fatalf("ssh defaults: %+v %v", c, err)
	}
	for _, tt := range []struct{ raw, scheme string }{
		{`not json`, "https"},
		{`{}`, "https"},
		{`{"token":"a\nb"}`, "https"},
		{`{"token":"t","username":"a:b"}`, "https"},
		{`{"private_key":"k"}`, "ssh"},
		{`{"private_key":"k","known_hosts":"h","ssh_user":"-oProxyCommand=x"}`, "ssh"},
	} {
		_, err := parseCredentials(tt.raw, tt.scheme)
		var e *adaptersdk.Error
		if !errors.As(err, &e) || e.Code != adaptersdk.AdapterErrorCodeAuth {
			t.Errorf("%s %s: %v, want auth", tt.scheme, tt.raw, err)
		}
		if err != nil && strings.Contains(err.Error(), "-oProxyCommand") {
			t.Errorf("the error repeats the value: %v", err)
		}
	}
}

func TestParseCursor(t *testing.T) {
	t.Parallel()
	sha := strings.Repeat("a", 40)
	c, ok := parseCursor(".." + sha + "/7")
	if !ok || c.base != "" || c.head != sha || c.offset != 7 || c.String() != ".."+sha+"/7" {
		t.Fatalf("full selection cursor %+v %v", c, ok)
	}
	if c, ok := parseCursor(sha + ".." + sha + "/0"); !ok || c.base != sha {
		t.Fatalf("diff cursor %+v %v", c, ok)
	}
	for _, raw := range []string{"", sha, "-x.." + sha + "/1", ".." + sha + "/-1", "..HEAD/1", ".." + sha} {
		if _, ok := parseCursor(raw); ok {
			t.Errorf("cursor %q is accepted", raw)
		}
	}
}

func TestDocuments(t *testing.T) {
	t.Parallel()
	for path, want := range map[string]bool{
		"README.md": true, "docs/Guide.MARKDOWN": true, "api/openapi.yaml": true, "openapi.json": true,
		"specs/billing.openapi.yml": true, "swagger.yaml": true, "config.yaml": false, "main.go": false, "docs/a.md.txt": false,
	} {
		if _, got := docFormat(path); got != want {
			t.Errorf("docFormat(%q) = %v", path, got)
		}
	}
	for _, p := range []string{"-x.md", "a/../b.md", "a\nb.md", "", strings.Repeat("a", 1100) + ".md"} {
		if validDocPath(p) {
			t.Errorf("path %q is accepted", p)
		}
	}
}

func TestHistoryParserStopsWhenEveryPathIsFound(t *testing.T) {
	t.Parallel()
	a, b := strings.Repeat("1", 40), strings.Repeat("2", 40)
	stream := "\x1e" + a + " 2026-03-02T10:00:00+03:00\x00\ndocs/a.md\x00" +
		"\x1e" + b + " 2026-03-01T10:00:00+03:00\x00\ndocs/a.md\x00README.md\x00" + "\x1e" + strings.Repeat("3", 40) + " 2026-02-01T10:00:00Z\x00\nREADME.md\x00"
	found := map[string]commitInfo{}
	stopped := 0
	h := &historyParser{wanted: map[string]bool{"docs/a.md": true, "README.md": true}, found: found, complete: func() { stopped++ }}
	for i := 0; i < len(stream); i += 7 {
		if _, err := h.Write([]byte(stream[i:min(i+7, len(stream))])); err != nil {
			t.Fatal(err)
		}
	}
	h.flush()
	if found["docs/a.md"].sha != a || found["README.md"].sha != b {
		t.Fatalf("the newest commit of each path: %+v", found)
	}
	if want := time.Date(2026, 3, 2, 7, 0, 0, 0, time.UTC); !found["docs/a.md"].at.Equal(want) {
		t.Fatalf("time %v", found["docs/a.md"].at)
	}
	if stopped == 0 {
		t.Fatal("the parser does not stop the walk once every path is found")
	}
}
