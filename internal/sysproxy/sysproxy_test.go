package sysproxy

import (
	"net/http"
	"net/url"
	"testing"
)

const scutil = `<dictionary> {
  ExceptionsList : <array> {
    0 : *.local
    1 : 169.254/16
    2 : git.corp
  }
  FTPPassive : 1
  HTTPEnable : 1
  HTTPPort : 9004
  HTTPProxy : remote.example.com
  HTTPSEnable : 1
  HTTPSPort : 9004
  HTTPSProxy : remote.example.com
  SOCKSEnable : 0
}
`

func proxyOf(t *testing.T, c config, raw string) string {
	t.Helper()
	u, err := c.proxy(&url.URL{Scheme: "https", Host: raw})
	if err != nil {
		t.Fatal(err)
	}
	if u == nil {
		return ""
	}
	return u.String()
}

func TestScutil(t *testing.T) {
	c := parseScutil(scutil)
	for host, want := range map[string]string{
		"github.com":     "http://remote.example.com:9004",
		"printer.local":  "",
		"git.corp":       "",
		"localhost:8080": "",
		"127.0.0.1":      "",
	} {
		if got := proxyOf(t, c, host); got != want {
			t.Errorf("%s: %q, want %q", host, got, want)
		}
	}
	if c := parseScutil("<dictionary> {\n  HTTPSEnable : 0\n  HTTPSProxy : off.example.com\n}\n"); proxyOf(t, c, "github.com") != "" {
		t.Error("a proxy turned off is used")
	}
	if c := parseScutil("<dictionary> {\n  SOCKSEnable : 1\n  SOCKSPort : 1080\n  SOCKSProxy : 127.0.0.1\n}\n"); proxyOf(t, c, "github.com") != "socks5://127.0.0.1:1080" {
		t.Errorf("socks: %+v", c)
	}
}

func TestWindows(t *testing.T) {
	c := parseWindows("proxy.example.com:8080", "<local>;*.corp.example.com")
	for host, want := range map[string]string{
		"github.com":            "http://proxy.example.com:8080",
		"intranet":              "",
		"git.corp.example.com":  "",
		"corp.example.com.evil": "http://proxy.example.com:8080",
	} {
		if got := proxyOf(t, c, host); got != want {
			t.Errorf("%s: %q, want %q", host, got, want)
		}
	}
	c = parseWindows("http=h:1;https=s:2;socks=k:3", "")
	if got := proxyOf(t, c, "github.com"); got != "http://s:2" {
		t.Errorf("per scheme: %q", got)
	}
}

func TestSet(t *testing.T) {
	defer Set("")
	req, _ := http.NewRequest("GET", "https://github.com/", nil)
	for raw, want := range map[string]string{
		"127.0.0.1:7890":          "http://127.0.0.1:7890",
		"socks5://127.0.0.1:1080": "socks5://127.0.0.1:1080",
		" https://p.example.com ": "https://p.example.com",
	} {
		if err := Set(raw); err != nil {
			t.Fatalf("%q: %v", raw, err)
		}
		if u, _ := Func(req); u == nil || u.String() != want {
			t.Errorf("%q: %v, want %s", raw, u, want)
		}
	}
	for _, raw := range []string{"ftp://p:21", "http://", "http://p:1/path"} {
		if Set(raw) == nil {
			t.Errorf("%q taken", raw)
		}
	}
}
