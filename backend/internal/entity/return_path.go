package entity

import (
	"net/url"
	"path"
	"strings"
)

func ReturnPath(raw string) string {
	const home = "/"
	if raw == "" || len(raw) > 2048 || raw[0] != '/' || strings.HasPrefix(raw, "//") || hasUnsafe(raw) {
		return home
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil || hasUnsafe(u.Path) ||
		strings.HasPrefix(u.Path, "//") || strings.HasPrefix(path.Clean(u.Path), "//") {
		return home
	}
	return raw
}

func hasUnsafe(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool { return r <= ' ' || r == '\\' || r == 0x7f })
}
