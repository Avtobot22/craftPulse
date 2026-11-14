package gitadapter

import (
	"errors"
	"net"
	"net/url"
	"regexp"
	"strings"
)

var (
	_defaultPorts = map[string]string{"https": "443", "ssh": "22"}
	// _hostName is a DNS name; an address in brackets is checked by net.ParseIP.
	_hostName = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$`)
	// _segment is one component of a repository path: no option, no traversal, no shell or URL syntax.
	_segment = regexp.MustCompile(`^[A-Za-z0-9_.~+@-]+$`)
	_sshUser = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]*$`)
)

var errRemote = errors.New("gitadapter: remote address is not supported")

// Server is the base address of a connection: the scheme and host a repository is reached at.
type Server struct {
	Scheme string
	Host   string
	Port   string
}

// Key is the instance key: the host in lower case and a port only when it is not the default of the scheme,
// so connections by ssh and https to the same host on default ports share the instance.
func (s Server) Key() string {
	if s.Port == _defaultPorts[s.Scheme] {
		return s.Host
	}
	return net.JoinHostPort(s.Host, s.Port)
}

// Remote is a repository on a server.
type Remote struct {
	Server
	// Path is the repository path without the leading slash and the .git suffix.
	Path string
	// ID is the normalized remote address, the identity of the repository (TZ 12.4).
	ID string
}

// URL is what git connects to; it never carries credentials, the user of ssh is not one.
func (r Remote) URL(sshUser string) string {
	u := url.URL{Scheme: r.Scheme, Host: hostPort(r.Server), Path: "/" + r.Path}
	if r.Scheme == "ssh" {
		u.User = url.User(sshUser)
	}
	return u.String()
}

// ParseServer reads base_url: https or ssh, a host, no user, query or fragment, at most a slash as the path.
func ParseServer(raw string) (Server, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "ssh") || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		u.Opaque != "" || (u.Path != "" && u.Path != "/") {
		return Server{}, errRemote
	}
	host, err := hostOf(u.Hostname())
	if err != nil {
		return Server{}, err
	}
	port := u.Port()
	if port == "" {
		port = _defaultPorts[u.Scheme]
	}
	if !validPort(port) {
		return Server{}, errRemote
	}
	return Server{Scheme: u.Scheme, Host: host, Port: port}, nil
}

// RemoteOf joins the server of a connection with a scope: the scope is the normalized address of a
// repository on that very server, so the credentials of a server never go to another host.
func RemoteOf(s Server, scopeID string) (Remote, error) {
	id, err := Normalize(scopeID)
	if err != nil || id != scopeID {
		return Remote{}, errRemote
	}
	key, path, ok := strings.Cut(id, "/")
	if !ok || key != s.Key() {
		return Remote{}, errRemote
	}
	return Remote{Server: s, Path: path, ID: id}, nil
}

// Normalize turns a remote address into the identity of its repository: the host in lower case, a port only
// when it is not the default, the path without the .git suffix and slashes at the ends, no scheme and no
// user. ssh://git@host/group/app.git, git@host:group/app.git and https://host/group/app give host/group/app.
func Normalize(raw string) (string, error) {
	scheme, rest := "", raw
	if s, r, ok := strings.Cut(raw, "://"); ok {
		scheme, rest = s, r
	}
	var hostport, path string
	switch {
	case scheme == "https" || scheme == "ssh":
		u, err := url.Parse(raw)
		if err != nil || u.RawQuery != "" || u.Fragment != "" {
			return "", errRemote
		}
		hostport, path = u.Host, u.Path
	case scheme != "":
		return "", errRemote
	default:
		// The scp form user@host:path, or the identity itself host[:port]/path.
		if at := strings.LastIndex(rest, "@"); at >= 0 {
			rest = rest[at+1:]
		}
		h, p, ok := strings.Cut(rest, ":")
		portPart, _, _ := strings.Cut(p, "/")
		if strings.HasPrefix(rest, "[") {
			hostport, path, _ = strings.Cut(rest, "/")
		} else if ok && !strings.Contains(h, "/") && !strings.HasPrefix(p, "/") && (!isPort(portPart) || !strings.Contains(p, "/")) {
			hostport, path, scheme = h, p, "ssh"
		} else {
			hostport, path, _ = strings.Cut(rest, "/")
		}
	}
	if at := strings.LastIndex(hostport, "@"); at >= 0 {
		hostport = hostport[at+1:]
	}
	host, port := hostport, ""
	if h, p, err := net.SplitHostPort(hostport); err == nil {
		host, port = h, p
	}
	host, err := hostOf(host)
	if err != nil {
		return "", err
	}
	key := host
	if port != "" && port != _defaultPorts[scheme] && (scheme != "" || !isDefaultPort(port)) {
		if !validPort(port) {
			return "", errRemote
		}
		key = net.JoinHostPort(host, port)
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	if !validPath(path) {
		return "", errRemote
	}
	return key + "/" + path, nil
}

func hostOf(raw string) (string, error) {
	host := strings.ToLower(strings.TrimSuffix(strings.Trim(raw, "[]"), "."))
	if ip := net.ParseIP(host); ip != nil {
		return ip.String(), nil
	}
	if host == "" || len(host) > 253 || !_hostName.MatchString(host) {
		return "", errRemote
	}
	return host, nil
}

func validPath(p string) bool {
	if p == "" || len(p) > 200 {
		return false
	}
	for seg := range strings.SplitSeq(p, "/") {
		if !_segment.MatchString(seg) || seg == "." || seg == ".." || strings.HasPrefix(seg, "-") {
			return false
		}
	}
	return true
}

func validPort(p string) bool {
	if !isPort(p) {
		return false
	}
	return p != "0"
}

func isPort(p string) bool {
	if p == "" || len(p) > 5 {
		return false
	}
	for _, c := range p {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func isDefaultPort(p string) bool { return p == "22" || p == "443" }

// validSSHUser keeps the user of ssh to a plain name: it goes into the address git passes to ssh.
func validSSHUser(u string) bool { return len(u) <= 64 && _sshUser.MatchString(u) }
