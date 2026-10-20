// Helpers builds the helper for every platform MyGit reaches over SSH,
// gzipped as a release publishes them, and signs each with the key of the
// app's updates when MYGO_UPDATER_PRIVATE_KEY has it.
//
//	go run ./internal/tools/helpers [-o dir]
//
// The folder is where the app keeps the helpers it downloads, so pointing
// MYGIT_HELPER_DIR at it tries helpers built here.
package main

import (
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"mygit/internal/remote"
)

var platforms = []remote.Platform{
	{OS: "linux", Arch: "amd64"}, {OS: "linux", Arch: "arm64"},
	{OS: "darwin", Arch: "amd64"}, {OS: "darwin", Arch: "arm64"},
	{OS: "windows", Arch: "amd64"}, {OS: "windows", Arch: "arm64"},
}

func main() {
	out := flag.String("o", "build/helpers", "the folder to write them in")
	flag.Parse()
	key := os.Getenv("MYGO_UPDATER_PRIVATE_KEY")
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	tmp, err := os.MkdirTemp("", "mygit-helpers")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmp)
	for _, p := range platforms {
		exe := filepath.Join(tmp, p.OS+"-"+p.Arch)
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", exe, "./cmd/mygit-helper")
		cmd.Env = append(os.Environ(), "GOOS="+p.OS, "GOARCH="+p.Arch, "CGO_ENABLED=0")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			log.Fatalf("building for %s: %v", p, err)
		}
		bin, err := os.ReadFile(exe)
		if err != nil {
			log.Fatal(err)
		}
		var gz bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&gz, gzip.BestCompression)
		zw.Write(bin)
		zw.Close()
		path := filepath.Join(*out, remote.Asset(p))
		if err := os.WriteFile(path, gz.Bytes(), 0o644); err != nil {
			log.Fatal(err)
		}
		signed := ""
		if key != "" {
			sig, err := remote.Sign(gz.Bytes(), key)
			if err != nil {
				log.Fatal(err)
			}
			if err := os.WriteFile(path+".sig", []byte(sig+"\n"), 0o644); err != nil {
				log.Fatal(err)
			}
			signed = ", signed"
		}
		fmt.Printf("%s  %.1f MB%s\n", path, float64(gz.Len())/(1<<20), signed)
	}
}
