package localapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

const TokenHeader = "X-LSYL-Panel-Token"
const maxAPIRequestBytes = 1 << 20

type Gate struct {
	host   string
	origin string
	token  string
}

func New(panelURL string) (*Gate, error) {
	u, err := url.Parse(panelURL)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" {
		return nil, fmt.Errorf("panel URL must use 127.0.0.1 and an explicit port")
	}
	if _, _, err := net.SplitHostPort(u.Host); err != nil {
		return nil, fmt.Errorf("panel URL has an invalid host: %w", err)
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return nil, fmt.Errorf("generate panel token: %w", err)
	}
	return &Gate{host: u.Host, origin: u.Scheme + "://" + u.Host, token: hex.EncodeToString(secret[:])}, nil
}

func (g *Gate) Token() string { return g.token }

// Script is injected by WebView2 before page scripts, never served over HTTP.
func (g *Gate) Script() string {
	origin, _ := json.Marshal(g.origin)
	token, _ := json.Marshal(g.token)
	return "if (location.origin === " + string(origin) + ") { Object.defineProperty(window, '__lsylPanelToken', {value: " + string(token) + "}); }"
}

func (g *Gate) Protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != g.host {
			http.Error(w, "invalid panel host", http.StatusForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			if r.Header.Get(TokenHeader) != g.token {
				http.Error(w, "panel authorization required", http.StatusForbidden)
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" && origin != g.origin {
				http.Error(w, "invalid panel origin", http.StatusForbidden)
				return
			}
			if r.ContentLength > maxAPIRequestBytes {
				http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxAPIRequestBytes)
		}
		next.ServeHTTP(w, r)
	})
}
