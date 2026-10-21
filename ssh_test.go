package main

import "testing"

// Git's ssh tries the keys set in order, the default one first, and
// leaves git alone when none is set.
func TestGitSSHCommand(t *testing.T) {
	oldKeys, oldDefault := prefs.SSHKeys, prefs.SSHDefault
	defer func() { prefs.SSHKeys, prefs.SSHDefault = oldKeys, oldDefault }()
	prefs.SSHKeys, prefs.SSHDefault = []string{}, ""
	if got := gitSSHCommand(); got != "" {
		t.Errorf("without keys %q", got)
	}
	prefs.SSHKeys = []string{"/home/u/.ssh/id_ed25519", "/home/u/.ssh/work key", "/home/u/.ssh/id_rsa"}
	prefs.SSHDefault = "/home/u/.ssh/work key"
	want := `ssh -i "/home/u/.ssh/work key" -i "/home/u/.ssh/id_ed25519" -i "/home/u/.ssh/id_rsa"`
	if got := gitSSHCommand(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	prefs.SSHDefault = "/home/u/.ssh/id_rsa"
	want = `ssh -i "/home/u/.ssh/id_rsa" -i "/home/u/.ssh/id_ed25519" -i "/home/u/.ssh/work key"`
	if got := gitSSHCommand(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
