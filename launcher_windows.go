package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"

	loc "mygit/locales"
)

// launcherDir is where the command goes: a folder of the user's own,
// added to their PATH, as no installer's folder is.
func launcherDir() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return "", fmt.Errorf("LOCALAPPDATA is not set")
	}
	return filepath.Join(base, "MyGit", "bin"), nil
}

// launcherScript is mygit.cmd: it opens the folder given, else the
// terminal's, in the running app, and returns at once.
func launcherScript(exe string) string {
	return fmt.Sprintf("@echo off\r\nrem %s: mygit [folder] opens the folder's repository.\r\n"+
		"if \"%%~1\"==\"\" (start \"\" \"%s\" \"%%CD%%\") else (start \"\" \"%s\" \"%%~f1\")\r\n", launcherMark, exe, exe)
}

func writeLauncher(exe string) (path, msg string, err error) {
	dir, err := launcherDir()
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	path = filepath.Join(dir, "mygit.cmd")
	if err := os.WriteFile(path, []byte(launcherScript(exe)), 0o644); err != nil {
		return "", "", err
	}
	if err := editUserPath(func(dirs []string) []string {
		if indexFold(dirs, dir) < 0 {
			dirs = append(dirs, dir)
		}
		return dirs
	}); err != nil {
		return "", "", err
	}
	return path, loc.LauncherDoneNewTerminal(path), nil
}

func removeLaunchers() ([]string, error) {
	dir, err := launcherDir()
	if err != nil {
		return nil, err
	}
	var removed []string
	path := filepath.Join(dir, "mygit.cmd")
	if ours(path) {
		if err := os.Remove(path); err != nil {
			return nil, err
		}
		removed = append(removed, path)
	}
	err = editUserPath(func(dirs []string) []string {
		if i := indexFold(dirs, dir); i >= 0 {
			dirs = append(dirs[:i], dirs[i+1:]...)
		}
		return dirs
	})
	return removed, err
}

func indexFold(dirs []string, dir string) int {
	for i, d := range dirs {
		if strings.EqualFold(filepath.Clean(d), filepath.Clean(dir)) {
			return i
		}
	}
	return -1
}

// editUserPath changes the user's PATH, kept in the registry, and tells
// the programs running that the environment changed, as the System
// settings do, so that terminals opened after it have it.
func editUserPath(edit func([]string) []string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, "Environment", registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	old, kind, err := k.GetStringValue("Path")
	if err != nil && err != registry.ErrNotExist {
		return err
	}
	var dirs []string
	for _, d := range strings.Split(old, ";") {
		if d != "" {
			dirs = append(dirs, d)
		}
	}
	value := strings.Join(edit(dirs), ";")
	if value == old {
		return nil
	}
	if kind == registry.SZ {
		err = k.SetStringValue("Path", value)
	} else {
		err = k.SetExpandStringValue("Path", value)
	}
	if err != nil {
		return err
	}
	broadcastEnvironment()
	return nil
}

func broadcastEnvironment() {
	const hwndBroadcast, wmSettingChange, smtoAbortIfHung = 0xffff, 0x001A, 0x0002
	env, _ := syscall.UTF16PtrFromString("Environment")
	send := syscall.NewLazyDLL("user32.dll").NewProc("SendMessageTimeoutW")
	var result uintptr
	send.Call(hwndBroadcast, wmSettingChange, 0, uintptr(unsafe.Pointer(env)), smtoAbortIfHung, 5000, uintptr(unsafe.Pointer(&result)))
}
