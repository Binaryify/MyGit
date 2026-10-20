// Package sysproxy has the app's downloads go through the proxy the system
// is set to, as browsers do. Go's own HTTP client reads only the
// environment's HTTPS_PROXY and the like, which an app opened from Finder
// or the Start menu doesn't have: where the internet is reached through a
// proxy only, its downloads would time out.
package sysproxy

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
)

// Install has http.DefaultTransport, which http.DefaultClient uses, go
// through Func's proxy.
func Install() {
	if t, ok := http.DefaultTransport.(*http.Transport); ok {
		t.Proxy = Func
	}
}

// manual is the proxy the user set, nil to go by the environment and the
// system.
var manual atomic.Pointer[url.URL]

// Set sets the proxy every download goes through, "" for the
// environment's or the system's: http://host:port, https:// or socks5://.
func Set(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		manual.Store(nil)
		return nil
	}
	u, err := Parse(raw)
	if err != nil {
		return err
	}
	manual.Store(u)
	return nil
}

// Parse reads a proxy's address, the scheme http:// when none is written.
func Parse(raw string) (*url.URL, error) {
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || strings.Trim(u.Path, "/") != "" {
		return nil, errors.New("not a proxy's address")
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
		return u, nil
	}
	return nil, errors.New("a proxy is http://, https:// or socks5://")
}

// Func is the proxy for a request: the one set, the environment's, or
// the system's, nil for none.
func Func(req *http.Request) (*url.URL, error) {
	if u := manual.Load(); u != nil {
		return u, nil
	}
	if u, err := http.ProxyFromEnvironment(req); u != nil || err != nil {
		return u, err
	}
	return settings().proxy(req.URL)
}

// config is a system's proxies: by scheme, host:port, and the hosts that
// go without them.
type config struct {
	http, https, socks string
	bypass             []string // host patterns: example.com, *.local, <local>
}

func (c config) proxy(u *url.URL) (*url.URL, error) {
	host := u.Hostname()
	if host == "" || c.bypassed(host) {
		return nil, nil
	}
	switch {
	case u.Scheme == "https" && c.https != "":
		return url.Parse("http://" + c.https)
	case u.Scheme == "http" && c.http != "":
		return url.Parse("http://" + c.http)
	case c.socks != "":
		return url.Parse("socks5://" + c.socks)
	}
	return nil, nil
}

// bypassed tells whether host goes without the proxy: this computer, or
// one of the hosts listed.
func (c config) bypassed(host string) bool {
	host = strings.ToLower(host)
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}
	for _, p := range c.bypass {
		p = strings.ToLower(strings.TrimSpace(p))
		switch {
		case p == "":
		case p == "<local>": // Windows': a name without a dot
			if !strings.Contains(host, ".") {
				return true
			}
		case strings.HasPrefix(p, "*."):
			if strings.HasSuffix(host, p[1:]) {
				return true
			}
		case strings.HasPrefix(p, "*"):
			if strings.HasSuffix(host, p[1:]) {
				return true
			}
		case host == p:
			return true
		}
	}
	return false
}
