// Package askpass answers git's and ssh's prompts for a username, a
// password, a token or a passphrase in the app's window, as git can't ask
// in a terminal the app doesn't have.
//
// Git runs the program GIT_ASKPASS (ssh, SSH_ASKPASS) names with the
// prompt, and reads the answer it prints. That program is the app's own
// executable: started with the address of the running app in its
// environment, it hands the prompt over and prints what the user typed.
package askpass

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const (
	addrVar = "MYGIT_ASKPASS"     // the running app's socket
	tagVar  = "MYGIT_ASKPASS_TAG" // which window asked, a repository's folder
)

type request struct {
	Tag, Prompt string
}

type reply struct {
	Answer string
	OK     bool
}

// Asker asks the user a prompt for a window and returns the answer, false
// when the user cancels. It is called on a goroutine of its own.
type Asker func(tag, prompt string) (string, bool)

// Server takes the prompts of the git commands the app runs.
type Server struct {
	ln  net.Listener
	exe string
}

// Listen starts taking prompts, which ask answers.
func Listen(ask Asker) (*Server, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(os.TempDir(), "mygit-askpass-"+strconv.Itoa(os.Getpid())+".sock")
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	s := &Server{ln: ln, exe: exe}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serve(conn, ask)
		}
	}()
	return s, nil
}

func serve(conn net.Conn, ask Asker) {
	defer conn.Close()
	var req request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		return
	}
	answer, ok := ask(req.Tag, req.Prompt)
	_ = json.NewEncoder(conn).Encode(reply{answer, ok})
}

// Env is the environment that makes git, and ssh under it, ask the app
// for a window's prompts.
func (s *Server) Env(tag string) []string {
	return []string{
		"GIT_ASKPASS=" + s.exe,
		"SSH_ASKPASS=" + s.exe,
		"SSH_ASKPASS_REQUIRE=force",
		addrVar + "=" + s.ln.Addr().String(),
		tagVar + "=" + tag,
	}
}

// Close stops taking prompts and removes the socket.
func (s *Server) Close() error { return s.ln.Close() }

// Main answers the prompt the app was started with as an askpass
// program, and reports whether it was: then the program should exit with
// the code it returns.
func Main(args []string) (code int, isAskpass bool) {
	addr := os.Getenv(addrVar)
	if addr == "" {
		return 0, false
	}
	prompt := ""
	if len(args) > 1 {
		prompt = args[1]
	}
	answer, err := Ask(addr, os.Getenv(tagVar), prompt)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mygit askpass:", err)
		return 1, true
	}
	fmt.Println(answer)
	return 0, true
}

// ErrCanceled says the user didn't answer.
var ErrCanceled = errors.New("canceled")

// Ask hands a prompt to the app at addr and waits for the user's answer.
func Ask(addr, tag, prompt string) (string, error) {
	conn, err := net.DialTimeout("unix", addr, 5*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(request{tag, prompt}); err != nil {
		return "", err
	}
	var r reply
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&r); err != nil {
		return "", err
	}
	if !r.OK {
		return "", ErrCanceled
	}
	return r.Answer, nil
}
