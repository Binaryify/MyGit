package remote

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestConfigHosts(t *testing.T) {
	home := t.TempDir()
	files := map[string]string{
		".ssh/config": `# mine
Include ~/.orbstack/ssh/config
Include conf.d/*

Host work
  HostName 10.0.0.2
Host win windows
Host *.internal !bad
Host=box "my box"
Match host work
  User ada
host work
`,
		".orbstack/ssh/config": "Host orb\n  Port 32222\n",
		".ssh/conf.d/a":        "Host pi\nInclude config\n", // includes itself in the end
	}
	for name, text := range files {
		p := filepath.Join(home, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := ConfigHosts(home)
	want := []string{"orb", "pi", "work", "win", "windows", "box", "my box"}
	if !slices.Equal(got, want) {
		t.Errorf("hosts = %q, want %q", got, want)
	}
	if hosts := ConfigHosts(t.TempDir()); len(hosts) != 0 {
		t.Errorf("no config: %q", hosts)
	}
}
