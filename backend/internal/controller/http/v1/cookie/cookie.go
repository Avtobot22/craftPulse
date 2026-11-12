package cookie

import (
	"net/http"
	"time"
)

// Names carry the __Host- prefix: the browser accepts them only with Secure, Path=/ and no Domain (TZ 13.1, 13.2).
const (
	Session = "__Host-cp_session"
	Login   = "__Host-cp_login"
)

func Set(w http.ResponseWriter, name, value string, lifetime time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   int(lifetime / time.Second),
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func Clear(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Path:     "/",
		MaxAge:   -1,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
