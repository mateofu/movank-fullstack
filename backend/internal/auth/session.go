package auth

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

const SessionCookie = "movank_session"

func (a *Authenticator) Origin(r *http.Request) string {
	if a.origin != "" {
		return a.origin
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (a *Authenticator) SameOrigin(r *http.Request) bool {
	return r.Header.Get("Origin") == a.Origin(r)
}

func (a *Authenticator) SetSession(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	age := int(time.Until(expires).Seconds())
	if token == "" {
		age = -1
	}
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: token, Path: "/v1", HttpOnly: true, Secure: strings.HasPrefix(a.Origin(r), "https://"), SameSite: http.SameSiteStrictMode, MaxAge: age, Expires: expires})
}

func authError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, "{\"error\":%q}\n", code)
}
