// Mygit-helper runs git for MyGit on another computer, which starts it
// there over SSH.
//
//	mygit-helper serve   take MyGit's calls on standard input
package main

import (
	"fmt"
	"os"

	"mygit/internal/askpass"
	"mygit/internal/helper"
)

func main() {
	// Run by git for a prompt, hand it to the helper, which asks MyGit.
	if code, ok := askpass.Main(os.Args); ok {
		os.Exit(code)
	}
	if len(os.Args) != 2 || os.Args[1] != "serve" {
		fmt.Fprintln(os.Stderr, "usage: mygit-helper serve")
		os.Exit(2)
	}
	if err := helper.Serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "mygit-helper:", err)
		os.Exit(1)
	}
}
