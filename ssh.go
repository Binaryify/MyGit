package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/egoist/mygo"

	"mygit/internal/askpass"
	"mygit/internal/git"
	"mygit/internal/helper"
	"mygit/internal/remote"
	loc "mygit/locales"
)

// askServer takes git's and ssh's prompts on this computer, once started.
var askServer *askpass.Server

// parseLocation reads a repository on another computer, host:/path/to/repo,
// the host as ssh takes it (user@host, or an alias of ~/.ssh/config). A
// path under the home folder there may start with ~/ or not at all. A
// Windows drive, C:\…, is a local path.
func parseLocation(s string) (host, dir string, ok bool) {
	s = strings.TrimSpace(s)
	i := strings.Index(s, ":")
	if i <= 1 || strings.ContainsAny(s[:i], `/\ `) {
		return "", "", false
	}
	host, dir = s[:i], s[i+1:]
	switch {
	case dir == "~" || dir == "":
		dir = "." // ssh starts in the home folder
	case strings.HasPrefix(dir, "~/"):
		dir = dir[2:]
	}
	return host, dir, true
}

// hostConn is the helper started on a computer, shared by the windows of
// its repositories, and started again when the connection ended.
type hostConn struct {
	mu       sync.Mutex
	conn     *remote.Conn
	platform remote.Platform // as last reached, which saves asking again
}

var (
	hostsMu sync.Mutex
	hosts   = map[string]*hostConn{}
)

// connect is host's helper, started if it isn't running.
func connect(ctx context.Context, host string) (*remote.Conn, error) {
	hostsMu.Lock()
	h := hosts[host]
	if h == nil {
		h = &hostConn{}
		hosts[host] = h
	}
	hostsMu.Unlock()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conn != nil {
		select {
		case <-h.conn.Done():
			_ = h.conn.Close()
			h.conn = nil
		default:
			return h.conn, nil
		}
	}
	c, err := sshDialer().Dial(ctx, host, h.platform)
	if err != nil {
		return nil, err
	}
	h.conn, h.platform = c, c.Platform
	return c, nil
}

// closeHosts ends the helpers, as the app quits.
func closeHosts() {
	hostsMu.Lock()
	defer hostsMu.Unlock()
	for _, h := range hosts {
		if h.conn != nil {
			_ = h.conn.Close()
		}
	}
}

func sshDialer() remote.SSH {
	cache, _ := os.UserCacheDir()
	store := remote.Store{
		Dir:     filepath.Join(cache, "MyGit", "helper", mygo.App.Version()),
		BaseURL: "https://github.com/Binaryify/MyGit/releases/download/v" + mygo.App.Version(),
	}
	if dir := os.Getenv("MYGIT_HELPER_DIR"); dir != "" { // helpers built here, to try them
		store.Dir = dir
	}
	return remote.SSH{
		Store: store,
		Env: func(host string) []string {
			if askServer == nil {
				return nil
			}
			return askServer.Env(host)
		},
		Ask: func(host, dir, prompt string) (string, bool) {
			return askUser(git.Location(host, dir), prompt)
		},
	}
}

// gitSSHCommand is ssh trying the keys set for git's remote operations,
// the default one first; "" when none is set, leaving git to its own.
func gitSSHCommand() string {
	keys := orderedSSHKeys()
	if len(keys) == 0 {
		return ""
	}
	args := []string{"ssh"}
	for _, k := range keys {
		args = append(args, "-i", strconv.Quote(k))
	}
	return strings.Join(args, " ")
}

// orderedSSHKeys is the keys set with the default one first.
func orderedSSHKeys() []string {
	keys := slices.Clone(prefs.SSHKeys)
	if i := slices.Index(keys, prefs.SSHDefault); i > 0 {
		def := keys[i]
		keys = append(keys[:i], keys[i+1:]...)
		keys = append([]string{def}, keys...)
	}
	return keys
}

// gitEnv adds to the environment of git run here: the askpass's, and the
// ssh command trying the keys set.
func gitEnv(dir string) []string {
	var env []string
	if askServer != nil {
		env = append(env, askServer.Env(dir)...)
	}
	if cmd := gitSSHCommand(); cmd != "" {
		env = append(env, "GIT_SSH_COMMAND="+cmd)
	}
	return env
}

// remoteHost runs git on another computer through its helper, starting it
// again when the connection ended. It is the git.Runner of repositories
// there.
type remoteHost struct{ host string }

func (h remoteHost) conn(ctx context.Context) (*remote.Conn, error) {
	c, err := connect(ctx, h.host)
	if err != nil {
		return nil, errors.New(loc.ConnectionLost(h.host, err.Error()))
	}
	return c, nil
}

// lost says the connection ended in the app's words.
func (h remoteHost) lost(err error) error {
	if errors.Is(err, helper.ErrClosed) {
		return errors.New(loc.ConnectionLost(h.host, err.Error()))
	}
	return err
}

func (h remoteHost) Exec(ctx context.Context, dir string, env []string, stdin io.Reader, stdout, stderr io.Writer, args ...string) error {
	c, err := h.conn(ctx)
	if err != nil {
		return err
	}
	return h.lost(c.Exec(ctx, dir, env, stdin, stdout, stderr, args...))
}

func (h remoteHost) ReadFile(ctx context.Context, p string) ([]byte, error) {
	c, err := h.conn(ctx)
	if err != nil {
		return nil, err
	}
	b, err := c.ReadFile(ctx, p)
	return b, h.lost(err)
}

func (h remoteHost) WriteFile(ctx context.Context, p string, data []byte) error {
	c, err := h.conn(ctx)
	if err != nil {
		return err
	}
	return h.lost(c.WriteFile(ctx, p, data))
}

func (h remoteHost) RemoveAll(ctx context.Context, p string) error {
	c, err := h.conn(ctx)
	if err != nil {
		return err
	}
	return h.lost(c.RemoveAll(ctx, p))
}

func (h remoteHost) Exists(ctx context.Context, p string) bool {
	c, err := h.conn(ctx)
	return err == nil && c.Exists(ctx, p)
}

func (h remoteHost) Join(dir, rel string) string { return path.Join(dir, rel) }

func (h remoteHost) Host() string { return h.host }

func (h remoteHost) Watch(ctx context.Context, dir string, changed func()) error {
	c, err := h.conn(ctx)
	if err != nil {
		return err
	}
	return h.lost(c.Watch(ctx, dir, changed))
}

// openRemote opens a repository on another computer, or the repositories
// in the folders under dir, starting its helper.
var openRemote = func(ctx context.Context, host, dir string) (string, []*git.Repo, error) {
	if _, err := connect(ctx, host); err != nil {
		return "", nil, err
	}
	return openFolders(ctx, remoteHost{host}, listOn(host), dir)
}

// addRemoteRecent puts a repository on another computer first among those
// opened lately.
func addRemoteRecent(location string) {
	r := slices.DeleteFunc(slices.Clone(prefs.Remotes), func(s string) bool { return s == location })
	prefs.Remotes = append([]string{location}, r...)
	if len(prefs.Remotes) > 10 {
		prefs.Remotes = prefs.Remotes[:10]
	}
	prefs.save()
}
