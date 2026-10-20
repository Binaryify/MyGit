// Mygit-agent runs git for MyGit on another computer, which starts it
// there over SSH.
//
//	mygit-agent serve   take MyGit's calls on standard input
package main

import (
	"fmt"
	"os"

	"mygit/internal/agent"
	"mygit/internal/askpass"
)

func main() {
	// Run by git for a prompt, hand it to the agent, which asks MyGit.
	if code, ok := askpass.Main(os.Args); ok {
		os.Exit(code)
	}
	if len(os.Args) != 2 || os.Args[1] != "serve" {
		fmt.Fprintln(os.Stderr, "usage: mygit-agent serve")
		os.Exit(2)
	}
	if err := agent.Serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "mygit-agent:", err)
		os.Exit(1)
	}
}
