package askpass

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestAsk(t *testing.T) {
	s, err := Listen(func(tag, prompt string) (string, bool) {
		if strings.HasPrefix(prompt, "Password") {
			return "", false
		}
		return tag + ":" + prompt, true
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	addr := ""
	for _, kv := range s.Env("/repo") {
		if v, ok := strings.CutPrefix(kv, addrVar+"="); ok {
			addr = v
		}
	}
	if got, err := Ask(addr, "/repo", "Username for 'https://example.com': "); err != nil || got != "/repo:Username for 'https://example.com': " {
		t.Fatalf("answer %q, %v", got, err)
	}
	if _, err := Ask(addr, "/repo", "Password for 'https://ada@example.com': "); err != ErrCanceled {
		t.Fatalf("canceled prompt: %v", err)
	}
}

// The test binary is the askpass program git runs, as the app is.
func TestMain(m *testing.M) {
	if code, ok := Main(os.Args); ok {
		os.Exit(code)
	}
	os.Exit(m.Run())
}

// git asks the app for a username and a password through the program.
func TestGitAsks(t *testing.T) {
	var prompts []string
	s, err := Listen(func(tag, prompt string) (string, bool) {
		prompts = append(prompts, prompt)
		if strings.HasPrefix(prompt, "Username") {
			return "ada", true
		}
		return "s3cret", true
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	cmd := exec.Command("git", "-c", "credential.helper=", "credential", "fill")
	cmd.Env = append(append(os.Environ(), "GIT_TERMINAL_PROMPT=0"), s.Env("/repo")...)
	cmd.Stdin = strings.NewReader("protocol=https\nhost=example.com\n\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(string(out), "username=ada") || !strings.Contains(string(out), "password=s3cret") || len(prompts) != 2 {
		t.Fatalf("output %q, prompts %q", out, prompts)
	}
}
