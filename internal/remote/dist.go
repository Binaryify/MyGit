// Package remote reaches repositories on other computers: it starts the
// agent there over SSH, sending it first when it isn't there yet.
package remote

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// PublicKey signs the agents published with a release: the app's update
// key, updates.publicKey of mygo.json.
const PublicKey = "q5C+iL6+Y6YsRMOB9qGLiQyaz7RW3lwtF9eqh4q10fI="

var publicKey = PublicKey // another in tests

// Platform is a system and a processor, as Go names them: linux/amd64.
type Platform struct{ OS, Arch string }

func (p Platform) String() string { return p.OS + "/" + p.Arch }

// Asset is the name of the agent for p in a release, gzipped; its
// signature is Asset + ".sig".
func Asset(p Platform) string { return "mygit-agent-" + p.OS + "-" + p.Arch + ".gz" }

// Store keeps the agents of a version of the app on this computer,
// downloading each the first time a computer of its platform is reached.
type Store struct {
	Dir     string // where they are kept, for the version
	BaseURL string // where they are published: …/releases/download/v1.2.3
}

// Binary is the agent for p, downloaded if it isn't kept yet. Agents are
// verified with PublicKey as they are downloaded.
func (s Store) Binary(ctx context.Context, p Platform) ([]byte, error) {
	path := filepath.Join(s.Dir, Asset(p))
	gz, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if gz, err = s.download(ctx, p); err == nil {
			_ = os.MkdirAll(s.Dir, 0o755)
			err = os.WriteFile(path, gz, 0o644)
		}
	}
	if err != nil {
		return nil, err
	}
	r, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}

func (s Store) download(ctx context.Context, p Platform) ([]byte, error) {
	gz, err := get(ctx, s.BaseURL+"/"+Asset(p))
	if err != nil {
		return nil, fmt.Errorf("downloading the agent for %s: %w", p, err)
	}
	sig, err := get(ctx, s.BaseURL+"/"+Asset(p)+".sig")
	if err != nil {
		return nil, fmt.Errorf("downloading the agent for %s: %w", p, err)
	}
	if err := Verify(gz, string(sig)); err != nil {
		return nil, err
	}
	return gz, nil
}

// Verify checks the signature of an agent: of its SHA-256 digest, in
// base64, as updates are signed.
func Verify(data []byte, signature string) error {
	pub, _ := base64.StdEncoding.DecodeString(publicKey)
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(signature))
	sum := sha256.Sum256(data)
	if err != nil || !ed25519.Verify(ed25519.PublicKey(pub), sum[:], sig) {
		return errors.New("the agent downloaded is not signed with the app's key")
	}
	return nil
}

// Sign signs an agent with the private key of the app's updates, base64.
func Sign(data []byte, privateKey string) (string, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(privateKey))
	if err != nil || len(key) != ed25519.PrivateKeySize {
		return "", errors.New("invalid private key")
	}
	sum := sha256.Sum256(data)
	return base64.StdEncoding.EncodeToString(ed25519.Sign(ed25519.PrivateKey(key), sum[:])), nil
}

func get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}
