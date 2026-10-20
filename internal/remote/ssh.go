package remote

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mygit/internal/agent"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// SSH reaches computers with the system's ssh, so its config, keys, agent
// and known hosts apply as in a terminal.
type SSH struct {
	Store Store
	// Env is added to the environment of ssh, as the askpass that asks
	// for a password or a passphrase in a window, for the host.
	Env func(host string) []string
	// Ask answers git's prompts on the computer, for the folder git runs
	// in there.
	Ask func(host, dir, prompt string) (string, bool)
}

// Conn is an agent started on a computer.
type Conn struct {
	*agent.Client
	Platform Platform
	cmd      *exec.Cmd
	stdin    io.Closer
}

// Close ends the connection and the agent.
func (c *Conn) Close() error {
	c.stdin.Close()
	return c.cmd.Wait()
}

// Dial starts the agent on host, sending it there first if it isn't yet.
// Known is host's platform as last reached, which saves asking.
func (s SSH) Dial(ctx context.Context, host string, known Platform) (*Conn, error) {
	p := known
	if p.OS == "" {
		var err error
		if p, err = s.platform(ctx, host); err != nil {
			return nil, err
		}
	}
	bin, err := s.Store.Binary(ctx, p)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(bin)
	dir := ".mygit/agent-" + hex.EncodeToString(sum[:6]) // in the home folder, where ssh starts
	c, err := s.start(ctx, host, p, dir)
	if err == nil {
		return c, nil
	}
	if known.OS != "" { // the computer may have changed
		if p2, perr := s.platform(ctx, host); perr == nil && p2 != p {
			return s.Dial(ctx, host, p2)
		}
	}
	if err := s.upload(ctx, host, p, dir, bin); err != nil {
		return nil, err
	}
	return s.start(ctx, host, p, dir)
}

// remote is the agent's path in dir for p's shell, which on Windows is
// cmd or PowerShell: backslashes, which both read, and .\ to run it.
func remote(p Platform, dir string) string {
	if p.OS == "windows" {
		return `.\` + strings.ReplaceAll(dir, "/", `\`) + `\mygit-agent.exe`
	}
	return dir + "/mygit-agent"
}

// start runs the agent in dir, waiting for it until ctx ends; it then
// runs until closed.
func (s SSH) start(ctx context.Context, host string, p Platform, dir string) (*Conn, error) {
	cmd := s.command(host, remote(p, dir)+" serve")
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	started := make(chan struct{})
	defer close(started)
	go func() {
		select {
		case <-ctx.Done():
			_ = cmd.Process.Kill()
		case <-started:
		}
	}()
	c, err := agent.NewClient(host, out, in, func(dir, prompt string) (string, bool) {
		if s.Ask == nil {
			return "", false
		}
		return s.Ask(host, dir, prompt)
	})
	if err != nil {
		in.Close()
		_ = cmd.Wait()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, errors.New(msg)
		}
		return nil, err
	}
	return &Conn{Client: c, Platform: p, cmd: cmd, stdin: in}, nil
}

// command is ssh running line on host.
func (s SSH) command(host, line string) *exec.Cmd {
	return s.cmd(host, "ssh", "-T", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=4", "--", host, line)
}

// cmd is ssh or scp for host. The connections to a host share one where
// ssh can, so a password is asked once and later ones start at once;
// Windows' ssh can't.
func (s SSH) cmd(host, name string, args ...string) *exec.Cmd {
	if runtime.GOOS != "windows" {
		args = append([]string{"-o", "ControlMaster=auto", "-o", "ControlPath=/tmp/mygit-%C", "-o", "ControlPersist=60"}, args...)
	}
	cmd := exec.Command(name, args...)
	cmd.Env = os.Environ()
	if s.Env != nil {
		cmd.Env = append(cmd.Env, s.Env(host)...)
	}
	hideWindow(cmd)
	return cmd
}

// output runs line on host and returns what it prints.
func (s SSH) output(ctx context.Context, host, line string, stdin io.Reader) (string, error) {
	cmd := s.command(host, line)
	var stdout, stderr bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, &stdout, &stderr
	if err := run(ctx, cmd); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return stdout.String(), errors.New(msg)
		}
		return stdout.String(), err
	}
	return stdout.String(), nil
}

// run runs cmd until it ends or ctx does.
func run(ctx context.Context, cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		<-done
		return ctx.Err()
	}
}

// platform asks host what it runs: uname answers on Unix, and Windows'
// shells, cmd or PowerShell, each expand one of the two variables.
func (s SSH) platform(ctx context.Context, host string) (Platform, error) {
	out, err := s.output(ctx, host, "uname -sm", nil)
	if err == nil {
		if p, ok := parseUname(out); ok {
			return p, nil
		}
	}
	if out, werr := s.output(ctx, host, "echo %PROCESSOR_ARCHITECTURE% $env:PROCESSOR_ARCHITECTURE", nil); werr == nil {
		for _, f := range strings.Fields(out) {
			switch strings.ToUpper(f) {
			case "AMD64":
				return Platform{"windows", "amd64"}, nil
			case "ARM64":
				return Platform{"windows", "arm64"}, nil
			}
		}
	}
	if err != nil {
		return Platform{}, err
	}
	return Platform{}, fmt.Errorf("%s runs a system MyGit doesn't support: %s", host, strings.TrimSpace(out))
}

// parseUname reads uname's line, the last: a shell's startup files may
// print before it.
func parseUname(out string) (Platform, bool) {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	f := strings.Fields(lines[len(lines)-1])
	if len(f) != 2 {
		return Platform{}, false
	}
	p := Platform{OS: strings.ToLower(f[0])}
	switch f[1] {
	case "x86_64", "amd64":
		p.Arch = "amd64"
	case "aarch64", "arm64":
		p.Arch = "arm64"
	}
	return p, (p.OS == "linux" || p.OS == "darwin") && p.Arch != ""
}

// upload puts the agent in dir on host: through ssh's input on Unix,
// and with scp on Windows, whose shells can't write their input to a file.
func (s SSH) upload(ctx context.Context, host string, p Platform, dir string, bin []byte) error {
	if p.OS != "windows" {
		a := remote(p, dir)
		_, err := s.output(ctx, host, fmt.Sprintf("mkdir -p %s && cat > %s.tmp && chmod +x %s.tmp && mv -f %s.tmp %s", dir, a, a, a, a), bytes.NewReader(bin))
		return err
	}
	_, _ = s.output(ctx, host, "mkdir "+strings.ReplaceAll(dir, "/", `\`), nil) // there already, it says so
	tmp, err := os.CreateTemp("", "mygit-agent-*.exe")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(bin)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	// A path with a drive, C:/…, would read as a host: scp runs where it is.
	cmd := s.cmd(host, "scp", "-q", "--", filepath.Base(tmp.Name()), host+":"+dir+"/mygit-agent.exe")
	cmd.Dir = filepath.Dir(tmp.Name())
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := run(ctx, cmd); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return errors.New(msg)
		}
		return err
	}
	return nil
}
