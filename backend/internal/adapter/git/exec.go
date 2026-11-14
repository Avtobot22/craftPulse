package gitadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

const (
	_path        = "/usr/local/bin:/usr/bin:/bin"
	_stderrLimit = 16 << 10
	_waitDelay   = 5 * time.Second
)

// _safeConfig is given to every git command; global and system configuration are never read. Nothing of
// the repository runs: no hooks, no filters, no submodules, no protocol but https and ssh, no redirect
// to an address that was not checked. The memory of a command stays within the limit of the container.
var _safeConfig = []string{
	"core.hooksPath=/dev/null",
	"core.fsmonitor=false",
	"core.askPass=",
	"init.templateDir=",
	"protocol.allow=never",
	"protocol.https.allow=always",
	"protocol.ssh.allow=always",
	"protocol.file.allow=never",
	"protocol.ext.allow=never",
	"submodule.recurse=false",
	"fetch.recurseSubmodules=false",
	"filter.lfs.smudge=",
	"filter.lfs.process=",
	"filter.lfs.required=false",
	"http.followRedirects=false",
	"transfer.fsckObjects=true",
	"gc.auto=0",
	"maintenance.auto=false",
	"fetch.writeCommitGraph=false",
	"core.packedGitLimit=64m",
	"core.packedGitWindowSize=16m",
	"core.deltaBaseCacheLimit=16m",
	"pack.windowMemory=64m",
	"pack.threads=1",
	"credential.helper=",
}

// credentials are the fields of the connection form, one JSON value of the collect purpose: a read-only
// token for https or a private key with the pinned host keys for ssh.
type credentials struct {
	Token      string `json:"token"`
	Username   string `json:"username"`
	PrivateKey string `json:"private_key"`
	KnownHosts string `json:"known_hosts"`
	SSHUser    string `json:"ssh_user"`
}

// parseCredentials checks the fields the scheme needs; an error names a field, never a value.
func parseCredentials(raw, scheme string) (credentials, error) {
	var c credentials
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return credentials{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeAuth, "credentials are not a JSON object of the connection form")
	}
	switch scheme {
	case "https":
		if c.Token == "" || strings.ContainsAny(c.Token, "\r\n") {
			return credentials{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeAuth, "https needs the field token")
		}
		if c.Username == "" {
			c.Username = "oauth2"
		}
		if strings.ContainsAny(c.Username, ":\r\n") {
			return credentials{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeAuth, "the field username is not a plain name")
		}
	case "ssh":
		if c.PrivateKey == "" || c.KnownHosts == "" {
			return credentials{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeAuth, "ssh needs the fields private_key and known_hosts")
		}
		if c.SSHUser == "" {
			c.SSHUser = "git"
		}
		if !validSSHUser(c.SSHUser) {
			return credentials{}, adaptersdk.NewError(adaptersdk.AdapterErrorCodeAuth, "the field ssh_user is not a plain name")
		}
	}
	return c, nil
}

// runner starts git with a fixed environment and a list of arguments, never through a shell.
type runner struct {
	git    string
	caFile string
	work   string
	// dial maps a checked address to the one git connects to; nil keeps it. Tests only.
	dial func(netip.Addr) netip.Addr
	// observe sees the arguments and the environment of every command. Tests only.
	observe func(args, env []string)
}

// session is what one remote operation needs: the files with the credentials, which exist only for the
// operation, and the address checked before it, which git is pinned to.
type session struct {
	dir  string
	env  []string
	conf []string
	url  string
}

// open prepares a remote operation; local commands use an empty session without credentials.
func (r *runner) open(rem Remote, c credentials, addr netip.Addr) (*session, error) {
	dir, err := os.MkdirTemp(r.work, "call-")
	if err != nil {
		return nil, fmt.Errorf("gitadapter: session: %w", err)
	}
	s := &session{dir: dir, env: baseEnv(dir), url: rem.URL(c.SSHUser)}
	if r.dial != nil {
		addr = r.dial(addr)
	}
	switch rem.Scheme {
	case "https":
		err = s.https(rem, c, addr, r.caFile)
	case "ssh":
		err = s.ssh(rem, c, addr)
	}
	if err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
}

func (s *session) https(rem Remote, c credentials, addr netip.Addr, caFile string) error {
	// The store helper reads the token from a file only the adapter user can read.
	line := url.URL{Scheme: "https", User: url.UserPassword(c.Username, c.Token), Host: hostPort(rem.Server)}
	file := filepath.Join(s.dir, "credentials")
	if err := os.WriteFile(file, []byte(line.String()+"\n"), 0o600); err != nil {
		return fmt.Errorf("gitadapter: credentials file: %w", err)
	}
	s.conf = append(s.conf, "credential.helper=store --file="+file, "credential.useHttpPath=false")
	if net.ParseIP(rem.Host) == nil {
		s.conf = append(s.conf, "http.curloptResolve="+rem.Host+":"+rem.Port+":"+literal(addr))
	}
	if caFile != "" {
		s.conf = append(s.conf, "http.sslCAInfo="+caFile)
	}
	return nil
}

func (s *session) ssh(rem Remote, c credentials, addr netip.Addr) error {
	key, hosts := filepath.Join(s.dir, "key"), filepath.Join(s.dir, "known_hosts")
	if err := os.WriteFile(key, []byte(strings.TrimRight(c.PrivateKey, "\n")+"\n"), 0o600); err != nil {
		return fmt.Errorf("gitadapter: key file: %w", err)
	}
	if err := os.WriteFile(hosts, []byte(c.KnownHosts+"\n"), 0o600); err != nil {
		return fmt.Errorf("gitadapter: known hosts file: %w", err)
	}
	// The alias is the name known_hosts has for the host, as ssh-keyscan writes it.
	alias := rem.Host
	if rem.Port != _defaultPorts["ssh"] {
		alias = "[" + rem.Host + "]:" + rem.Port
	}
	// git runs this through a shell: it holds only paths of the session and checked values, each quoted.
	opts := []string{
		"ssh", "-F", "/dev/null", "-i", key, "-o", "IdentitiesOnly=yes", "-o", "IdentityAgent=none",
		"-o", "UserKnownHostsFile=" + hosts, "-o", "GlobalKnownHostsFile=/dev/null", "-o", "StrictHostKeyChecking=yes",
		"-o", "UpdateHostKeys=no", "-o", "HostKeyAlias=" + alias, "-o", "HostName=" + addr.Unmap().String(),
		"-o", "BatchMode=yes", "-o", "PasswordAuthentication=no", "-o", "KbdInteractiveAuthentication=no",
		"-o", "ProxyCommand=none", "-o", "ProxyJump=none", "-o", "PermitLocalCommand=no", "-o", "ControlMaster=no",
		"-o", "ControlPath=none", "-o", "ClearAllForwardings=yes", "-o", "ConnectTimeout=10",
	}
	for i, o := range opts {
		opts[i] = "'" + o + "'"
	}
	s.env = append(s.env, "GIT_SSH_COMMAND="+strings.Join(opts, " "), "GIT_SSH_VARIANT=ssh")
	return nil
}

// Close removes the files of the credentials.
func (s *session) Close() error { return os.RemoveAll(s.dir) }

// baseEnv is the whole environment of git: nothing of the adapter process reaches it.
func baseEnv(home string) []string {
	return []string{
		"PATH=" + _path, "HOME=" + home, "XDG_CONFIG_HOME=" + home, "LC_ALL=C",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_ATTR_NOSYSTEM=1",
		"GIT_ALLOW_PROTOCOL=https:ssh", "GIT_PROTOCOL_FROM_USER=0", "GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=/bin/false", "SSH_ASKPASS=/bin/false", "GIT_LFS_SKIP_SMUDGE=1", "GIT_NO_REPLACE_OBJECTS=1",
		"GIT_NO_LAZY_FETCH=1", "GIT_OPTIONAL_LOCKS=0",
	}
}

// run starts git -C dir with the safe configuration, the session and args, writing stdout to out.
func (r *runner) run(ctx context.Context, s *session, dir string, out io.Writer, args ...string) error {
	full := make([]string, 0, 2*len(_safeConfig)+2*len(s.conf)+len(args)+2)
	if dir != "" {
		full = append(full, "-C", dir)
	}
	for _, kv := range slices.Concat(_safeConfig, s.conf) {
		full = append(full, "-c", kv)
	}
	full = append(full, args...)
	cmd := exec.CommandContext(ctx, r.git, full...) //nolint:gosec // a fixed binary with a list of checked arguments, no shell
	cmd.Env, cmd.Dir, cmd.Stdout = s.env, s.dir, out
	stderr := &capped{left: _stderrLimit}
	cmd.Stderr = stderr
	// git starts ssh and remote helpers; the whole group stops with the command.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = _waitDelay
	if r.observe != nil {
		r.observe(full, cmd.Env)
	}
	started := time.Now()
	err := cmd.Run()
	ev := adaptersdk.Call{}.Log(ctx).Debug().Str("git", args[0]).Dur("duration", time.Since(started))
	if ru, ok := usage(cmd); ok {
		ev = ev.Int64("max_rss_bytes", ru.Maxrss*1024)
	}
	ev.Msg("git finished")
	if err == nil {
		return nil
	}
	if errors.Is(err, errOutputTooLarge) {
		return adaptersdk.InvalidResponseError("git %s output exceeds the limit", args[0])
	}
	return classify(ctx, args[0], stderr.String(), err)
}

// output runs git and returns its stdout up to max bytes.
func (r *runner) output(ctx context.Context, s *session, dir string, maxBytes int64, args ...string) ([]byte, error) {
	var buf bytes.Buffer
	err := r.run(ctx, s, dir, &limited{w: &buf, left: maxBytes}, args...)
	return buf.Bytes(), err
}

// classify maps a failed git command to a contract error by its exit and stderr; only the class reaches the
// core, stderr stays in the log of the adapter.
func classify(ctx context.Context, op, stderr string, err error) error {
	if cause := context.Cause(ctx); cause != nil && !errors.Is(cause, context.Canceled) && !errors.Is(cause, context.DeadlineExceeded) {
		return cause
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeTimeout, "git "+op+" did not finish in time")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	adaptersdk.Call{}.Log(ctx).Warn().Str("git", op).Str("stderr", firstLines(stderr, 3)).Err(err).Msg("git failed")
	msg := strings.ToLower(stderr)
	has := func(parts ...string) bool {
		for _, p := range parts {
			if strings.Contains(msg, p) {
				return true
			}
		}
		return false
	}
	switch {
	case has("authentication failed", "permission denied", "could not read username", "could not read password",
		"invalid username or password", "host key verification failed", "access denied", "the requested url returned error: 401",
		"the requested url returned error: 403", "no matching host key"):
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeAuth, "git "+op+" was refused by the remote")
	case has("repository not found", "does not appear to be a git repository", "not found", "does not exist",
		"the requested url returned error: 404"):
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeNotFound, "git "+op+": repository is not found")
	case has("timed out", "timeout"):
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeTimeout, "git "+op+": remote did not answer in time")
	case has("certificate", "ssl"):
		return adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "git "+op+": remote certificate is not trusted")
	}
	return adaptersdk.NewError(adaptersdk.AdapterErrorCodeUnavailable, "git "+op+" failed")
}

func usage(cmd *exec.Cmd) (*syscall.Rusage, bool) {
	if cmd.ProcessState == nil {
		return nil, false
	}
	ru, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage)
	return ru, ok
}

func firstLines(s string, n int) string {
	lines := strings.SplitN(strings.TrimSpace(s), "\n", n+1)
	return strings.Join(lines[:min(n, len(lines))], " | ")
}

func hostPort(s Server) string {
	host := s.Host
	if net.ParseIP(host) != nil && strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if s.Port == _defaultPorts[s.Scheme] {
		return host
	}
	return net.JoinHostPort(s.Host, s.Port)
}

func literal(a netip.Addr) string {
	a = a.Unmap()
	if a.Is6() {
		return "[" + a.String() + "]"
	}
	return a.String()
}

var errOutputTooLarge = errors.New("gitadapter: output exceeds the limit")

type limited struct {
	w    io.Writer
	left int64
}

func (l *limited) Write(p []byte) (int, error) {
	if int64(len(p)) > l.left {
		return 0, errOutputTooLarge
	}
	l.left -= int64(len(p))
	return l.w.Write(p)
}

// capped keeps the beginning of stderr and drops the rest, so a noisy command does not grow memory.
type capped struct {
	buf  bytes.Buffer
	left int
}

func (c *capped) Write(p []byte) (int, error) {
	if c.left > 0 {
		n := min(len(p), c.left)
		c.buf.Write(p[:n])
		c.left -= n
	}
	return len(p), nil
}

func (c *capped) String() string { return c.buf.String() }
