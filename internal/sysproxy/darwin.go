//go:build darwin

package sysproxy

import "os/exec"

// settings reads the proxies of System Settings, as scutil prints them.
func settings() config {
	out, err := exec.Command("/usr/sbin/scutil", "--proxy").Output()
	if err != nil {
		return config{}
	}
	return parseScutil(string(out))
}
