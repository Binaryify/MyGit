package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// checkUpload checks, before a build, that -upload has somewhere to upload
// to and the key that signs the updates: without it the build writes no
// manifests, and installed apps could not update to the release.
func checkUpload(c *Config) error {
	switch u := c.Updates; {
	case u != nil && u.S3 != nil:
		if _, err := newS3Client(u.S3); err != nil {
			return err
		}
	case u == nil || u.GitHub == "":
		return fmt.Errorf("-upload needs updates.github or updates.s3 in %s", c.configName())
	}
	key, err := c.signingKey()
	if err != nil {
		return fmt.Errorf("updates: %w", err)
	}
	if key == nil {
		return errors.New("-upload needs the update signing key, or installed apps could not update to this release: set MYGO_UPDATER_PRIVATE_KEY or updates.privateKey")
	}
	return nil
}

// publish uploads the artifacts of a build where updates point to: the
// bucket of updates.s3, else the GitHub release of this version.
func publish(c *Config, artifacts []string) error {
	if c.Updates != nil && c.Updates.S3 != nil {
		return publishS3(c, artifacts)
	}
	return publishGitHub(c, artifacts)
}

// uploads returns the artifacts of a build to publish: the installers and
// update files, and apart the update manifests, which go last so that they
// never point at a file that is not there.
func uploads(artifacts []string) (files, manifests []string, err error) {
	for _, a := range artifacts {
		info, err := os.Stat(a)
		if err != nil || info.IsDir() {
			continue // the macOS bundle ships in the disk image and the update
		}
		name := filepath.Base(a)
		switch {
		case strings.HasPrefix(name, "update-") && strings.HasSuffix(name, ".json"):
			manifests = append(manifests, a)
		case name == installScriptName, name == installPS1Name, name == checksumsName:
			// The same script for every target of a system.
			if !slices.ContainsFunc(files, func(f string) bool { return filepath.Base(f) == name }) {
				files = append(files, a)
			}
		case strings.HasSuffix(name, ".dmg"), strings.HasSuffix(name, ".exe") && strings.Contains(name, installerMark),
			strings.HasSuffix(name, ".deb"), strings.HasSuffix(name, ".tar.gz"), strings.HasSuffix(name, ".delta"):
			files = append(files, a)
		}
	}
	if len(files)+len(manifests) == 0 {
		return nil, nil, errors.New("nothing to upload")
	}
	return files, manifests, nil
}

// publishGitHub uploads the artifacts of a build to the GitHub release of
// this version, a draft created if needed, with the gh CLI. Installed apps
// see the update once the release is published.
func publishGitHub(c *Config, artifacts []string) error {
	if c.Updates == nil || c.Updates.GitHub == "" {
		return fmt.Errorf("-upload needs updates.github in %s", c.configName())
	}
	gh, err := exec.LookPath("gh")
	if err != nil {
		return errors.New("-upload needs the GitHub CLI (https://cli.github.com), signed in with gh auth login or GH_TOKEN")
	}
	repo, tag := c.Updates.GitHub, c.Updates.TagPrefix+c.Version
	files, manifests, err := uploads(artifacts)
	if err != nil {
		return err
	}
	// With a tag prefix, the repository's latest release is another's, which
	// the app's releases leave alone.
	latest := ""
	if c.Updates.tagged() {
		latest = " --latest=false"
	}
	if exec.Command(gh, "release", "view", tag, "--repo", repo).Run() == nil {
		// The release may hold the files of other builds, as of another
		// platform, whose checksums stay.
		if err := mergeReleaseChecksums(gh, repo, tag, files); err != nil {
			return err
		}
	} else {
		notes, _ := c.releaseNotes()
		t := con.start("Creating the draft release " + tag + " of " + repo)
		args := []string{"release", "create", tag, "--repo", repo, "--draft", "--title", tag, "--notes", notes}
		if latest != "" {
			args = append(args, "--latest=false")
		}
		if out, err := exec.Command(gh, args...).CombinedOutput(); err != nil {
			t.fail()
			return fmt.Errorf("gh release create: %v\n%s", err, out)
		}
		t.done("Created the draft release " + tag + " of " + repo)
	}
	for _, batch := range [][]string{files, manifests} {
		if len(batch) == 0 {
			continue
		}
		t := con.start("Uploading " + uploadNames(batch))
		args := append([]string{"release", "upload", tag, "--repo", repo, "--clobber"}, batch...)
		if out, err := exec.Command(gh, args...).CombinedOutput(); err != nil {
			t.fail()
			return fmt.Errorf("gh release upload: %v\n%s", err, out)
		}
		t.done("Uploaded " + uploadNames(batch))
	}
	// A version such as 1.3.0-beta.1 is published as a prerelease, which
	// only apps that take prereleases (Updater.SetPrereleases) update to.
	if v, _, _ := strings.Cut(c.Version, "+"); strings.Contains(v, "-") {
		latest += " --prerelease"
	}
	logf("Uploaded to the release %s of %s; publish it when every platform is there:\n  gh release edit %s --repo %s --draft=false%s", tag, repo, tag, repo, latest)
	return nil
}

// mergeReleaseChecksums merges the SHA256SUMS of the release tag, if it
// has one, into the SHA256SUMS among files.
func mergeReleaseChecksums(gh, repo, tag string, files []string) error {
	i := slices.IndexFunc(files, func(f string) bool { return filepath.Base(f) == checksumsName })
	if i < 0 {
		return nil
	}
	dir, err := os.MkdirTemp("", "mygo-checksums-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	out, err := exec.Command(gh, "release", "view", tag, "--repo", repo, "--json", "assets", "--jq", ".assets[].name").Output()
	if err != nil {
		return fmt.Errorf("gh release view %s: %v", tag, err)
	}
	if !slices.Contains(strings.Split(string(out), "\n"), checksumsName) {
		return nil // none yet
	}
	// Uploading without the lines of the other builds would leave their
	// files unchecked, and install.ps1 without its installer.
	if out, err := exec.Command(gh, "release", "download", tag, "--repo", repo, "--pattern", checksumsName, "--dir", dir).CombinedOutput(); err != nil {
		return fmt.Errorf("gh release download %s %s: %v\n%s", tag, checksumsName, err, out)
	}
	earlier, err := os.ReadFile(filepath.Join(dir, checksumsName))
	if err != nil {
		return err
	}
	return mergeChecksums(files[i], earlier, "")
}

// publishS3 uploads the artifacts of a build to the bucket of updates.s3,
// which updates.url serves. Installed apps see the update of a platform
// once its manifest is there.
func publishS3(c *Config, artifacts []string) error {
	bucket, err := newS3Client(c.Updates.S3)
	if err != nil {
		return err
	}
	files, manifests, err := uploads(artifacts)
	if err != nil {
		return err
	}
	if err := mergeS3Checksums(c, files); err != nil {
		return err
	}
	for _, f := range slices.Concat(files, manifests) {
		name := filepath.Base(f)
		t := con.start("Uploading " + name)
		t.set(sizeOf(f))
		if err := t.end(bucket.put(bucket.key(name), f, uploadHeader(name)), "Uploaded "+name); err != nil {
			return err
		}
	}
	logf("Uploaded to s3://%s/%s, which %s serves", bucket.bucket, bucket.prefix, c.Updates.URL)
	return nil
}

// mergeS3Checksums merges the SHA256SUMS that updates.url serves, of the
// builds uploaded before, as of another platform, into the SHA256SUMS
// among files, which would otherwise replace it without their lines. It
// leaves out the files of the earlier versions that this upload replaces,
// as all versions share the bucket.
func mergeS3Checksums(c *Config, files []string) error {
	i := slices.IndexFunc(files, func(f string) bool { return filepath.Base(f) == checksumsName })
	if i < 0 {
		return nil
	}
	u := c.updateFile(checksumsName, true)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "mygo/"+version)
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := (&http.Client{Timeout: time.Minute}).Do(req)
	if err != nil {
		return fmt.Errorf("reading the published %s: %w", checksumsName, err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound, http.StatusForbidden:
		return nil // none yet: a bucket that does not list its objects answers 403
	default:
		return fmt.Errorf("reading the published %s: GET %s: %s", checksumsName, u, resp.Status)
	}
	earlier, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return fmt.Errorf("reading the published %s: %w", checksumsName, err)
	}
	return mergeChecksums(files[i], earlier, c.Version)
}

// uploadHeader returns the headers of an uploaded file: its type and, for
// the files whose names stay from version to version, that caches must
// check them again.
func uploadHeader(name string) http.Header {
	typ, ok := contentTypes[filepath.Ext(name)]
	if !ok {
		typ = "application/octet-stream"
	}
	h := http.Header{"Content-Type": {typ}}
	if name == installScriptName || name == installPS1Name || name == checksumsName || strings.HasPrefix(name, "update-") && strings.HasSuffix(name, ".json") {
		h.Set("Cache-Control", "no-cache")
	}
	return h
}

var contentTypes = map[string]string{
	".dmg":  "application/x-apple-diskimage",
	".exe":  "application/vnd.microsoft.portable-executable",
	".deb":  "application/vnd.debian.binary-package",
	".gz":   "application/gzip",
	".json": "application/json",
	".sh":   "text/plain; charset=utf-8", // readable in a browser before piping it to sh
	".ps1":  "text/plain; charset=utf-8",
}

// uploadNames names the files of an upload.
func uploadNames(files []string) string {
	if len(files) == 1 {
		return filepath.Base(files[0])
	}
	return fmt.Sprintf("%d files", len(files))
}
