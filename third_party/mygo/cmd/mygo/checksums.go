package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// checksumsName is the file of the SHA-256 of the files a build publishes,
// in the format of sha256sum, which install.ps1 checks the Windows
// installer against, and which people can check their downloads with:
//
//	sha256sum -c --ignore-missing SHA256SUMS
//
// It names the files as the build does; GitHub turns the spaces of
// uploaded names into dots.
const checksumsName = "SHA256SUMS"

var checksumLine = regexp.MustCompile(`^([0-9a-f]{64}) [ *](.+)$`)

// writeChecksums writes SHA256SUMS into dir, with the SHA-256 of each file
// of artifacts that -upload publishes but the manifests, the same name
// once, and returns its path, or "" when there is none.
func writeChecksums(dir string, artifacts []string) (string, error) {
	files, _, err := uploads(artifacts)
	if err != nil || len(files) == 0 {
		return "", nil
	}
	sums := map[string]string{}
	for _, f := range files {
		name := filepath.Base(f)
		if name == checksumsName || sums[name] != "" {
			continue
		}
		sum, err := fileSHA256(f)
		if err != nil {
			return "", err
		}
		sums[name] = sum
	}
	path := filepath.Join(dir, checksumsName)
	return path, os.WriteFile(path, formatChecksums(sums), 0o644)
}

// mergeChecksums adds to the SHA256SUMS at path the lines of earlier, as
// those of an earlier upload to the same release, for the files it does
// not list. With version, the version of this upload, it leaves out the
// files of other versions that this upload replaces, as where the files
// of every version are side by side (updates.s3): a line whose name is
// that of a file listed at path with another version in place of version.
func mergeChecksums(path string, earlier []byte, version string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sums := parseChecksums(data)
	var replaced []*regexp.Regexp
	if version != "" {
		for name := range sums {
			if before, after, ok := strings.Cut(name, version); ok {
				replaced = append(replaced, regexp.MustCompile(`^`+regexp.QuoteMeta(before)+`\S+`+regexp.QuoteMeta(after)+`$`))
			}
		}
	}
	for name, sum := range parseChecksums(earlier) {
		if _, ok := sums[name]; ok || slices.ContainsFunc(replaced, func(re *regexp.Regexp) bool { return re.MatchString(name) }) {
			continue
		}
		sums[name] = sum
	}
	return os.WriteFile(path, formatChecksums(sums), 0o644)
}

func parseChecksums(data []byte) map[string]string {
	sums := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		if m := checksumLine.FindStringSubmatch(strings.TrimSuffix(line, "\r")); m != nil {
			sums[m[2]] = m[1]
		}
	}
	return sums
}

func formatChecksums(sums map[string]string) []byte {
	var b strings.Builder
	for _, name := range slices.Sorted(maps.Keys(sums)) {
		fmt.Fprintf(&b, "%s  %s\n", sums[name], name)
	}
	return []byte(b.String())
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
