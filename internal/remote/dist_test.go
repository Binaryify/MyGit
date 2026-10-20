package remote

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The helpers are signed with the app's update key.
func TestPublicKeyIsTheApps(t *testing.T) {
	b, err := os.ReadFile("../../mygo.json")
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct{ Updates struct{ PublicKey string } }
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Updates.PublicKey != PublicKey {
		t.Errorf("PublicKey = %s, mygo.json has %s", PublicKey, cfg.Updates.PublicKey)
	}
}

func TestStoreDownloadsSigned(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	old := publicKey
	publicKey = base64.StdEncoding.EncodeToString(pub)
	defer func() { publicKey = old }()

	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	zw.Write([]byte("#!helper"))
	zw.Close()
	sig, err := Sign(gz.Bytes(), base64.StdEncoding.EncodeToString(priv))
	if err != nil {
		t.Fatal(err)
	}
	p := Platform{"linux", "arm64"}
	served := map[string][]byte{"/" + Asset(p): gz.Bytes(), "/" + Asset(p) + ".sig": []byte(sig)}
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if b, ok := served[r.URL.Path]; ok {
			w.Write(b)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	s := Store{Dir: t.TempDir(), BaseURL: srv.URL}
	for range 2 { // the second time from where it's kept
		bin, err := s.Binary(context.Background(), p)
		if err != nil || string(bin) != "#!helper" {
			t.Fatalf("binary = %q, %v", bin, err)
		}
	}
	if hits != 2 {
		t.Errorf("%d downloads, want the helper and its signature once", hits)
	}

	// Another file under the signature isn't taken.
	served["/"+Asset(Platform{"linux", "amd64"})] = []byte("evil")
	served["/"+Asset(Platform{"linux", "amd64"})+".sig"] = []byte(sig)
	if _, err := s.Binary(context.Background(), Platform{"linux", "amd64"}); err == nil || !strings.Contains(err.Error(), "not signed") {
		t.Errorf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, Asset(Platform{"linux", "amd64"}))); err == nil {
		t.Error("a helper not signed was kept")
	}
}

func TestParseUname(t *testing.T) {
	for in, want := range map[string]Platform{
		"Linux x86_64\n":                     {"linux", "amd64"},
		"Linux aarch64\n":                    {"linux", "arm64"},
		"Darwin arm64\n":                     {"darwin", "arm64"},
		"hello from bashrc\nLinux aarch64\n": {"linux", "arm64"},
	} {
		if p, ok := parseUname(in); !ok || p != want {
			t.Errorf("%q: %v %v", in, p, ok)
		}
	}
	for _, in := range []string{"FreeBSD amd64", "Linux riscv64", "'uname' is not recognized"} {
		if _, ok := parseUname(in); ok {
			t.Errorf("%q read", in)
		}
	}
}

func TestRemotePath(t *testing.T) {
	if got := remote(Platform{"windows", "amd64"}, ".mygit/helper-ab"); got != `.\.mygit\helper-ab\mygit-helper.exe` {
		t.Error(got)
	}
	if got := remote(Platform{"linux", "amd64"}, ".mygit/helper-ab"); got != ".mygit/helper-ab/mygit-helper" {
		t.Error(got)
	}
}
