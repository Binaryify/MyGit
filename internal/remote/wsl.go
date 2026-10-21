package remote

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"unicode/utf16"
)

// wslPrefix starts the host of a distribution of WSL on this computer,
// wsl+Ubuntu, as VS Code names it too. Its helper runs through wsl.exe
// rather than ssh.
const wslPrefix = "wsl+"

// WSL is the distribution host names, when it is one of WSL's.
func WSL(host string) (distro string, ok bool) {
	distro, ok = strings.CutPrefix(host, wslPrefix)
	return distro, ok && distro != ""
}

// WSLHost is the host of a distribution of WSL.
func WSLHost(distro string) string { return wslPrefix + distro }

// wslCommand runs line in distro's home folder, where ssh would start.
func wslCommand(distro, line string) *exec.Cmd {
	cmd := exec.Command("wsl.exe", "-d", distro, "--cd", "~", "--exec", "sh", "-c", line)
	hideWindow(cmd)
	return cmd
}

// Distros lists the distributions of WSL on this computer, but Docker
// Desktop's own, which have no git; none off Windows or without WSL.
func Distros(ctx context.Context) []string {
	if runtime.GOOS != "windows" {
		return nil
	}
	cmd := exec.CommandContext(ctx, "wsl.exe", "--list", "--quiet")
	cmd.Env = append(os.Environ(), "WSL_UTF8=1")
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil // no WSL, or no distribution in it
	}
	var list []string
	for _, l := range strings.Split(decodeWSL(out), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "docker-desktop") {
			list = append(list, l)
		}
	}
	return list
}

// decodeWSL reads what wsl.exe prints: UTF-16 before it learned
// WSL_UTF8, UTF-8 since.
func decodeWSL(b []byte) string {
	b = bytes.TrimPrefix(b, []byte{0xff, 0xfe})
	if len(b) < 2 || bytes.IndexByte(b, 0) < 0 {
		return string(b)
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return string(utf16.Decode(u))
}
