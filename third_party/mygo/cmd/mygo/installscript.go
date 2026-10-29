package main

import (
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"unicode"

	"github.com/egoist/mygo/internal/update"
)

// The install script, install.sh, installs the app for the user, without
// root or a package manager, on Linux and, with updates, on macOS:
//
//	curl -fsSL https://github.com/me/my-app/releases/latest/download/install.sh | sh
//
// On Linux it installs the archive of the app in ~/.local/<name>.app, where
// the app can update itself, with the command of linux.command in
// ~/.local/bin when there is one (never over a file it did not make), and
// registers its desktop entry and file types (writeLinuxDesktop) with the
// paths of the install, as updates do again (update.RefreshDesktopEntry).
// It warns when WebKitGTK is missing, with the command that installs it,
// unless the app shows no web page (linux.webview false). It installs the
// archive it is given, else the one of its version next to it, else, with
// updates, the latest version, which the manifest of the machine's target
// names, after checking its size and its Ed25519 signature against
// updates.publicKey, as the app's updater does, with openssl pkeyutl
// (OpenSSL 3.0 or later). Without such an openssl it downloads nothing and
// says so: an archive given to it is not checked.
//
// On macOS it downloads the archive of the latest version that the manifest
// of the Mac's architecture (else of darwin-universal) names, Apple silicon
// even from a shell under Rosetta, checks its size and its Ed25519
// signature against updates.publicKey, as the app's updater does, quits
// the app if it runs, puts the bundle in /Applications,
// or ~/Applications when the user cannot write there or it is already
// there, and opens it. curl sets no quarantine, so the app opens without
// the prompt of Gatekeeper for a downloaded app. macOS has no tool that
// checks Ed25519 signatures (its LibreSSL has none), so the script hashes
// with openssl and shasum and does the arithmetic of the curve in
// JavaScript for Automation (osascript), which every Mac has.
//
// It reads the manifest as writeArchive writes it, indented with the fields
// of the version on their own lines.
//
// For tests, <NAME>_DOWNLOAD_URL (the slug of the app in capitals, such as
// MY_APP_DOWNLOAD_URL) is where the manifests are instead, and on macOS
// <NAME>_INSTALL_DIR the folder of the app and <NAME>_NO_OPEN=1 leaves it
// closed.

const installScriptName = "install.sh"

// writeInstallScript writes install.sh into dir and returns its path.
func writeInstallScript(c *Config, dir string) (string, error) {
	path := filepath.Join(dir, installScriptName)
	return path, os.WriteFile(path, []byte(installScript(c)), 0o755)
}

func installScript(c *Config) string {
	data := map[string]string{
		"Name":       c.Name,
		"Title":      strings.Join(strings.Fields(c.Name), " "),
		"Slug":       c.slug(),
		"Command":    c.Linux.Command,
		"Version":    c.Version,
		"Bundle":     c.executableName() + ".app",
		"Identifier": c.Identifier,
		"Env":        envName(c.slug()),
		"Releases":   "",
		"API":        "",
		"TagPrefix":  "",
		"PublicKey":  "",
		"WebKit":     "",
		"Updates":    "",
	}
	if c.Linux.webView() {
		data["WebKit"] = "yes"
	}
	switch u := c.Updates; {
	case u != nil && u.tagged():
		// The newest release tagged with the prefix, which the GitHub API
		// finds among the others.
		data["Releases"] = "https://github.com/" + u.GitHub + "/releases/download/"
		data["API"] = update.GitHubAPI + "/repos/" + u.GitHub + "/releases?per_page=100"
		data["TagPrefix"] = u.TagPrefix
		data["Curl"] = "curl -fsSL " + c.updateFile(installScriptName, false) + " | sh"
	case u != nil:
		data["Releases"] = c.updateFile("", true)
		data["Curl"] = "curl -fsSL " + c.updateFile(installScriptName, true) + " | sh"
	}
	if c.Updates != nil {
		// Without updates, the script installs only an archive given to
		// it, on Linux, and has none of the code that downloads, checks
		// signatures or installs on macOS.
		data["Updates"] = "yes"
		data["PublicKey"] = strings.TrimSpace(c.Updates.PublicKey)
	}
	var b strings.Builder
	if err := installScriptTemplate.Execute(&b, data); err != nil {
		panic(err)
	}
	return b.String()
}

// envName is the prefix of the environment variables of an install script
// of the app slug: MY_APP for my-app.
func envName(slug string) string {
	name := strings.Map(func(r rune) rune {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return unicode.ToUpper(r)
		}
		return '_'
	}, slug)
	if name == "" || name[0] >= '0' && name[0] <= '9' {
		name = "APP_" + name
	}
	return name
}

// shQuote quotes s for a POSIX shell.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

var installScriptTemplate = template.Must(template.New("").Funcs(template.FuncMap{"q": shQuote}).Parse(`#!/bin/sh
# Installs {{.Title}} for the current user, without root. On Linux: the app in
# ~/.local/{{.Slug}}.app{{with .Command}}, the {{.}} command in ~/.local/bin,{{end}} and its entry
# in the applications menu.{{if .PublicKey}} On macOS: the latest version, its signature
# checked, in /Applications, or ~/Applications when it cannot write there.{{end}}
# Made by mygo build.
#
{{- with .Curl}}
#   {{.}}
{{- end}}
#   sh install.sh [ARCHIVE]
#   sh install.sh --uninstall
set -eu

app_name={{q .Name}}
name={{q .Slug}}
# The command in ~/.local/bin that runs the app (linux.command), if any.
command_name={{q .Command}}
version={{q .Version}}
# Where the manifests of the latest version are, with updates: in the
# release of the newest tag with the prefix when the repository's releases
# (api) hold others too.
releases={{q .Releases}}
{{- if .Updates}}
api={{q .API}}
tag_prefix={{q .TagPrefix}}
# macOS: the bundle of the app, its identifier, and the Ed25519 public key
# of updates, which the archive it installs is signed with.
bundle={{q .Bundle}}
bundle_id={{q .Identifier}}
public_key={{q .PublicKey}}
# For tests only: where the manifests are instead, and on macOS the folder
# to install the app in and leaving it closed.
download_url="{{"${"}}{{.Env}}_DOWNLOAD_URL:-}"
install_dir="{{"${"}}{{.Env}}_INSTALL_DIR:-}"
no_open="{{"${"}}{{.Env}}_NO_OPEN:-}"
{{- end}}

usage() {
	cat <<EOF
Usage: sh install.sh [ARCHIVE | --uninstall]

Installs $app_name for you. On Linux, in ~/.local: ARCHIVE, else
$name-$version-<platform>.tar.gz next to this script{{if .Curl}}, else the latest
version, which it downloads{{end}}.{{if .PublicKey}} On macOS, the latest version, in
/Applications, or ~/Applications when you cannot write there.{{end}}
--uninstall removes it, and leaves its settings and data in place.
EOF
}

fail() {
	echo "install.sh: $*" >&2
	exit 1
}

fetch() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$1"
	else
		wget -qO- "$1"
	fi
}

main() {
	if [ $# -gt 1 ]; then
		usage >&2
		exit 2
	fi
	case "${1:-}" in
	-h | --help)
		usage
		return
		;;
	--uninstall) ;;
	-*)
		usage >&2
		exit 2
		;;
	esac
{{- if .Updates}}
	if [ -n "$download_url" ]; then
		releases="${download_url%/}/"
		api=
	fi
	if [ "$(uname -s)" = Darwin ]; then
		macos "$@"
	else
		linux "$@"
	fi
{{- else}}
	linux "$@"
{{- end}}
}

linux() {
	app_dir="$HOME/.local/$name.app"
	bin_dir="$HOME/.local/bin"
	data_home="${XDG_DATA_HOME:-$HOME/.local/share}"
	desktop_file="$data_home/applications/$name.desktop"
	mime_file="$data_home/mime/packages/$name.xml"
	if [ "${1:-}" = --uninstall ]; then
		uninstall
		return
	fi

	[ "$(uname -s)" = Linux ] || fail "$app_name installs on Linux{{if .PublicKey}} and macOS{{end}}"
	[ "$(id -u)" != 0 ] || fail "run this as the user to install $app_name for, not as root"
	# The desktop entry holds the path, which these characters would break.
	[ "$(printf '%s' "$app_dir" | tr -d '"$%\\\140\n')" = "$app_dir" ] ||
		fail "cannot install in $app_dir: the path has characters a desktop entry cannot hold"
	case "$(uname -m)" in
	x86_64 | amd64) target=linux-amd64 ;;
	aarch64 | arm64) target=linux-arm64 ;;
	*) fail "$app_name is not made for $(uname -m)" ;;
	esac

	work="$(mktemp -d)"
	staging="$HOME/.local/.$name.app.install"
	trap 'rm -rf "$work" "$staging"' EXIT
	trap 'exit 1' HUP INT TERM

	archive="${1:-}"
	if [ -z "$archive" ] && [ -f "$0" ] && [ -f "$(dirname -- "$0")/$name-$version-$target.tar.gz" ]; then
		archive="$(dirname -- "$0")/$name-$version-$target.tar.gz"
	fi
	if [ -z "$archive" ]; then
		[ -n "$releases" ] || fail "no $name-$version-$target.tar.gz next to this script: pass the archive to install"
{{- if .Updates}}
		command -v curl >/dev/null 2>&1 || command -v wget >/dev/null 2>&1 || fail "downloading $app_name needs curl or wget"
		if [ -n "$api" ]; then
			tag="$(latest_tag)" || fail "could not list the releases at $api"
			[ -n "$tag" ] || fail "no release of $app_name is published"
			manifest="$releases$tag/update-$target.json"
		else
			manifest="${releases}update-$target.json"
		fi
		# Fail before downloading when nothing here checks Ed25519.
		has_ed25519 || fail "checking the signature of $app_name needs OpenSSL 3.0 or later (the openssl command): install it, or download $name-<version>-$target.tar.gz and run sh install.sh with it"
		fetch "$manifest" >"$work/update.json" || fail "could not download $manifest"
		latest="$(field version)"
		url="$(field url)"
		size="$(field size)"
		signature="$(field signature)"
		[ -n "$url" ] || fail "$manifest names no archive"
		[ -n "$size" ] && [ -n "$signature" ] || fail "$manifest names no signed archive"
		echo "Downloading $app_name $latest"
		archive="$work/$name.tar.gz"
		fetch "$url" >"$archive" || fail "could not download $url"
		[ "$(wc -c <"$archive" | tr -d ' ')" = "$size" ] || fail "$url does not have the size of the manifest: not installing it"
		verify_openssl "$archive" "$signature" || fail "$url is not signed with the key of $app_name: not installing it"
{{- end}}
	fi
	[ -f "$archive" ] || fail "no archive at $archive"
	tar -tzf "$archive" >"$work/names" 2>/dev/null || fail "$archive is not a .tar.gz archive"
	if grep -Eq '^/|(^|/)\.\.(/|$)' "$work/names"; then
		fail "$archive holds files outside the app"
	fi

	echo "Installing $app_name in $app_dir"
	rm -rf "$staging"
	mkdir -p "$staging"
	tar -xzf "$archive" -C "$staging"
	[ -f "$staging/$name" ] && [ -x "$staging/$name" ] || fail "$archive is not $app_name: it holds no $name"
	# Replace the app as a whole, so that no file of another version stays.
	rm -rf "$app_dir"
	mv "$staging" "$app_dir"

	mkdir -p "$data_home/applications"
	# Earlier install scripts linked a command named after the app.
	if [ "$name" != "$command_name" ] && ours "$bin_dir/$name"; then
		rm -f "$bin_dir/$name"
	fi
	run=
	if [ -n "$command_name" ]; then
		bin_link="$bin_dir/$command_name"
		if { [ -e "$bin_link" ] || [ -L "$bin_link" ]; } && ! ours "$bin_link"; then
			echo "Leaving $bin_link alone: it is not $app_name's" >&2
		else
			mkdir -p "$bin_dir"
			ln -sf "$app_dir/$name" "$bin_link"
			if [ "$(command -v "$command_name" || true)" = "$bin_link" ]; then
				run=$command_name
			else
				run=$bin_link
			fi
		fi
	fi
	# The entry of the app, running it and showing its icon by their paths,
	# which updates of the app register again the same way.
	if [ -f "$app_dir/$name.desktop" ]; then
		while IFS= read -r line || [ -n "$line" ]; do
			case "$line" in
			"Exec=$name" | "Exec=$name "*) line="Exec=\"$app_dir/$name\"${line#"Exec=$name"}" ;;
			"Icon=$name") if [ -f "$app_dir/$name.png" ]; then line="Icon=$app_dir/$name.png"; fi ;;
			esac
			printf '%s\n' "$line"
		done <"$app_dir/$name.desktop" >"$desktop_file"
	fi
	if [ -f "$app_dir/$name.xml" ]; then
		mkdir -p "$data_home/mime/packages"
		cp "$app_dir/$name.xml" "$mime_file"
	else
		rm -f "$mime_file"
	fi
	update_databases

	if [ -n "$run" ]; then
		echo "Installed $app_name: open it from the applications menu, or run $run"
	else
		echo "Installed $app_name: open it from the applications menu"
	fi
{{- if .WebKit}}
	if ! has_webkit; then
		echo "$app_name needs WebKitGTK, which is not installed. Install it with:" >&2
		echo "  $(webkit_install_command)" >&2
	fi
{{- end}}
}
{{- if .Updates}}

# latest_tag prints the tag of the newest release tagged $tag_prefix that is
# neither a draft nor a prerelease. The GitHub API lists releases newest
# first, each with its tag_name, draft and prerelease fields in that order.
latest_tag() {
	page=1
	while [ "$page" -le 10 ]; do
		fetch "$api&page=$page" >"$work/releases.json" || return 1
		tr -d '\n' <"$work/releases.json" |
			grep -oE '"(tag_name|draft|prerelease)": *("[^"]*"|true|false)' >"$work/fields" || true
		tag="$(awk -v prefix="$tag_prefix" '
			/^"tag_name"/ { sub(/^"tag_name": *"/, ""); sub(/"$/, ""); tag = $0; draft = 0; next }
			/^"draft"/ { draft = /true$/; next }
			/^"prerelease"/ { if (!draft && /false$/ && index(tag, prefix) == 1) { print tag; exit } }
		' "$work/fields")"
		if [ -n "$tag" ]; then
			echo "$tag"
			return
		fi
		# A full page leads to the next.
		[ "$(grep -c '^"tag_name"' "$work/fields" || true)" -ge 100 ] || return 0
		page=$((page + 1))
	done
}

# macos installs the latest version on macOS, whose archive the manifest of
# the Mac's architecture, else of darwin-universal, names, after checking
# its size and signature; or with --uninstall removes the installed app.
macos() {
	[ "$(id -u)" != 0 ] || fail "run this as the user to install $app_name for, not as root"
	work="$(mktemp -d)"
	staging=
	trap 'rm -rf "$work"; [ -z "$staging" ] || rm -rf "$staging"' EXIT
	trap 'exit 1' HUP INT TERM
	if [ "${1:-}" = --uninstall ]; then
		macos_uninstall
		return
	fi
	[ -z "${1:-}" ] || fail "on macOS, install.sh installs the latest version, not an archive"
	[ -n "$releases" ] && [ -n "$public_key" ] || fail "$app_name installs on macOS from its disk image"
	command -v curl >/dev/null 2>&1 || fail "downloading $app_name needs curl"
	case "$(uname -m)" in
	arm64) arch=arm64 ;;
	x86_64) arch=amd64 ;;
	*) fail "$app_name is not made for $(uname -m)" ;;
	esac
	# A shell under Rosetta sees an Intel Mac; the Mac is Apple silicon.
	if [ "$arch" = amd64 ] && [ "$(sysctl -n sysctl.proc_translated 2>/dev/null)" = 1 ]; then
		arch=arm64
	fi
	if [ -n "$api" ]; then
		tag="$(latest_tag)" || fail "could not list the releases at $api"
		[ -n "$tag" ] || fail "no release of $app_name is published"
		base="$releases$tag/"
	else
		base="$releases"
	fi
	if fetch "${base}update-darwin-$arch.json" >"$work/update.json" 2>/dev/null; then
		target="darwin-$arch"
	elif fetch "${base}update-darwin-universal.json" >"$work/update.json" 2>/dev/null; then
		target=darwin-universal
	else
		fail "could not download ${base}update-darwin-$arch.json or update-darwin-universal.json"
	fi
	latest="$(field version)"
	url="$(field url)"
	size="$(field size)"
	signature="$(field signature)"
	[ -n "$url" ] && [ -n "$size" ] && [ -n "$signature" ] || fail "the manifest of $target names no signed archive"

	echo "Downloading $app_name $latest"
	archive="$work/$name.tar.gz"
	fetch "$url" >"$archive" || fail "could not download $url"
	[ "$(wc -c <"$archive" | tr -d ' ')" = "$size" ] || fail "$url does not have the size of the manifest: not installing it"
	verify "$archive" "$signature" || fail "$url is not signed with the key of $app_name: not installing it"
	tar -tzf "$archive" >"$work/names" 2>/dev/null || fail "$url is not a .tar.gz archive"
	if ! awk -v b="$bundle" '$0 != b && index($0, b "/") != 1 || /(^|\/)\.\.(\/|$)/ { bad = 1 } END { exit bad }' "$work/names"; then
		fail "$url holds files outside $bundle"
	fi

	dir="$(app_folder)"
	app="$dir/$bundle"
	if [ -e "$app" ] && [ "$(bundle_identifier "$app")" != "$bundle_id" ]; then
		fail "$app is another app than $app_name: not replacing it"
	fi
	mkdir -p "$dir"
	[ -w "$dir" ] || fail "cannot write to $dir"
	staging="$dir/.$bundle.install.$$"
	rm -rf "$staging"
	mkdir "$staging"
	tar -xzf "$archive" -C "$staging" || fail "could not extract $url"
	[ -d "$staging/$bundle/Contents/MacOS" ] && [ "$(bundle_identifier "$staging/$bundle")" = "$bundle_id" ] ||
		fail "$url is not $app_name"

	quit_app
	echo "Installing $app_name in $app"
	# Replace the app as a whole, with renames on the same volume: the
	# installed version is moved aside, and back if the new one cannot take
	# its place.
	if [ -e "$app" ]; then
		mv "$app" "$staging/previous.app" || fail "could not replace $app"
		if ! mv "$staging/$bundle" "$app"; then
			if ! mv "$staging/previous.app" "$app"; then
				# Keep it for the user rather than remove it on exit.
				previous="$staging/previous.app"
				staging=
				fail "could not replace $app: the installed version is in $previous"
			fi
			fail "could not replace $app"
		fi
	else
		mv "$staging/$bundle" "$app" || fail "could not move $app_name to $app"
	fi
	rm -rf "$staging"
	staging=
	if [ -z "$no_open" ]; then
		open "$app" </dev/null >/dev/null 2>&1 || echo "Could not open $app" >&2
	fi
	echo "Installed $app_name $latest in $app"
}

macos_uninstall() {
	dir="$(app_folder)"
	app="$dir/$bundle"
	if [ ! -d "$app" ] || [ "$(bundle_identifier "$app")" != "$bundle_id" ]; then
		if [ -n "$install_dir" ]; then
			fail "$app_name is not installed in $install_dir"
		fi
		fail "$app_name is not installed in /Applications or ~/Applications"
	fi
	quit_app
	rm -rf "$app" || fail "could not remove $app"
	echo "Uninstalled $app_name. Its settings and data are still in place."
}

# app_folder prints the folder of the app on macOS: the one that holds it,
# else /Applications, else, when the user cannot write there,
# ~/Applications.
app_folder() {
	if [ -n "$install_dir" ]; then
		echo "$install_dir"
		return
	fi
	for dir in /Applications "$HOME/Applications"; do
		if [ -d "$dir/$bundle" ] && [ "$(bundle_identifier "$dir/$bundle")" = "$bundle_id" ]; then
			echo "$dir"
			return
		fi
	done
	if [ -w /Applications ]; then
		echo /Applications
	else
		echo "$HOME/Applications"
	fi
}

bundle_identifier() {
	/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$1/Contents/Info.plist" 2>/dev/null || true
}

# field prints a field of the manifest.
field() {
	sed -n -e 's/^  "'"$1"'": "\([^"]*\)",\{0,1\}$/\1/p' -e 's/^  "'"$1"'": \([0-9][0-9]*\),\{0,1\}$/\1/p' "$work/update.json" | head -n 1
}

running() {
	[ "$(osascript -e "application id \"$bundle_id\" is running" </dev/null 2>/dev/null)" = true ]
}

# quit_app quits the app if it runs, as quitting it from its menu does, and
# waits for it to quit.
quit_app() {
	running || return 0
	echo "Quitting $app_name"
	osascript -e "tell application id \"$bundle_id\" to quit" </dev/null >/dev/null 2>&1 || true
	i=0
	while running; do
		i=$((i + 1))
		[ "$i" -le 60 ] || fail "$app_name is still running: quit it and run this again"
		sleep 1
	done
}

hex() {
	od -An -v -tx1 | tr -d ' \n'
}

# verify checks that the archive $1 has the Ed25519 signature $2 of its
# SHA-256 digest by the key of updates, as the app's updater does: with S
# and R of the signature, A the key and k the SHA-512 of R, A and the
# digest, that [S]B - [k]A is R (RFC 8032), computed in JavaScript for
# Automation, since macOS has no tool that checks Ed25519 signatures.
verify() {
	sig="$(printf '%s' "$2" | base64 -D 2>/dev/null | hex)" || return 1
	key="$(printf '%s' "$public_key" | base64 -D 2>/dev/null | hex)" || return 1
	[ "${#sig}" = 128 ] && [ "${#key}" = 64 ] || return 1
	k="$({
		printf '%s' "$2" | base64 -D | head -c 32
		printf '%s' "$public_key" | base64 -D
		openssl dgst -sha256 -binary <"$1"
	} | shasum -a 512 | cut -c 1-128)" || return 1
	verifier >"$work/verify.js"
	[ "$(osascript -l JavaScript "$work/verify.js" "$key" "$(echo "$sig" | cut -c 1-64)" "$(echo "$sig" | cut -c 65-128)" "$k" </dev/null 2>/dev/null)" = ok ]
}

# verify_openssl checks, as verify does, that the archive $1 has the
# Ed25519 signature $2 of its SHA-256 digest by the key of updates, with
# OpenSSL 3.0 or later (pkeyutl -rawin), as on Linux. The key in PEM is the raw key
# after the DER prefix of an Ed25519 public key (302a300506032b6570032100),
# whose 12 bytes are 16 characters of base64.
verify_openssl() {
	printf '%s' "$public_key" | grep -Eq '^[A-Za-z0-9+/]{43}=$' || return 1
	printf '%s' "$2" | grep -Eq '^[A-Za-z0-9+/]{86}==$' || return 1
	openssl dgst -sha256 -binary <"$1" >"$work/digest" || return 1
	ed25519_verify "MCowBQYDK2VwAyEA$public_key" "$2" "$work/digest"
}

# ed25519_verify checks with OpenSSL that the file $3 has the Ed25519
# signature $2, in base64, by the public key $1, in base64 DER.
ed25519_verify() {
	printf '%s\n' '-----BEGIN PUBLIC KEY-----' "$1" '-----END PUBLIC KEY-----' >"$work/key.pem"
	printf '%s' "$2" | openssl base64 -d -A >"$work/signature" 2>/dev/null || return 1
	[ "$(wc -c <"$work/signature" | tr -d ' ')" = 64 ] || return 1
	openssl pkeyutl -verify -pubin -inkey "$work/key.pem" -rawin -in "$3" -sigfile "$work/signature" >/dev/null 2>&1
}

# has_ed25519 tells whether openssl checks Ed25519 signatures, as OpenSSL
# 1.1.1 and LibreSSL do not, with test 2 of RFC 8032, whose message is the
# byte 0x72.
has_ed25519() {
	command -v openssl >/dev/null 2>&1 || return 1
	printf r >"$work/rfc8032"
	ed25519_verify MCowBQYDK2VwAyEAPUAXw+hDiVqStwqnTRt+vJyYLM8uxJaMwM1V8Sr0Zgw= \
		kqAJqfDUyrhyDoILX2QlQKKye1QWUD+Ps3YiI+vbadoIWsHkPhWZbkWPNhPQ8R2MOHsurrQwKu6wDSkWErsMAA== "$work/rfc8032"
}

# verifier prints the JavaScript that checks an Ed25519 signature: its
# arguments are the key, R, S and k in hexadecimal, as little-endian
# numbers, and it prints ok when [S]B - [k]A encodes as R.
verifier() {
	cat <<'EOF'
function run(argv) {
	var p = 2n ** 255n - 19n
	var l = 2n ** 252n + 27742317777372353535851937790883648493n
	function mod(a) { a %= p; return a < 0n ? a + p : a }
	function pow(b, e) {
		var r = 1n
		for (b = mod(b); e > 0n; e >>= 1n) {
			if (e & 1n) r = r * b % p
			b = b * b % p
		}
		return r
	}
	function inv(a) { return pow(a, p - 2n) }
	var d = mod(-121665n * inv(121666n)), sqrtm1 = pow(2n, (p - 1n) / 4n)
	function num(h) {
		var n = 0n
		for (var i = h.length - 2; i >= 0; i -= 2) n = n << 8n | BigInt(parseInt(h.substr(i, 2), 16))
		return n
	}
	function hex(n) {
		var h = ''
		for (var i = 0; i < 32; i++, n >>= 8n) h += (n & 255n).toString(16).padStart(2, '0')
		return h
	}
	// Points in extended coordinates (X, Y, Z, T), added with the formulas
	// of Hisil, Wong, Carter and Dawson for a = -1.
	function add(a, b) {
		var A = mod((a[1] - a[0]) * (b[1] - b[0])), B = mod((a[1] + a[0]) * (b[1] + b[0]))
		var C = mod(2n * d * a[3] * b[3]), D = mod(2n * a[2] * b[2])
		var E = B - A, F = D - C, G = D + C, H = B + A
		return [mod(E * F), mod(G * H), mod(F * G), mod(E * H)]
	}
	function mul(a, n) {
		var r = [0n, 1n, 1n, 0n]
		for (; n > 0n; n >>= 1n) {
			if (n & 1n) r = add(r, a)
			a = add(a, a)
		}
		return r
	}
	function point(h) {
		if (!/^[0-9a-f]{64}$/.test(h)) return null
		var y = num(h), sign = y >> 255n
		y &= (1n << 255n) - 1n
		if (y >= p) return null
		var u = mod(y * y - 1n), v = mod(d * y * y + 1n)
		var x = pow(u * inv(v), (p + 3n) / 8n)
		if (mod(v * x * x - u) != 0n) {
			x = mod(x * sqrtm1)
			if (mod(v * x * x - u) != 0n) return null
		}
		if (x == 0n && sign == 1n) return null
		if ((x & 1n) != sign) x = p - x
		return [x, y, 1n, mod(x * y)]
	}
	function encode(a) {
		var z = inv(a[2]), x = mod(a[0] * z), y = mod(a[1] * z)
		return hex(y | (x & 1n) << 255n)
	}
	var A = point(argv[0])
	if (!A || !/^[0-9a-f]{64}$/.test(argv[1]) || !/^[0-9a-f]{64}$/.test(argv[2]) || !/^[0-9a-f]{128}$/.test(argv[3])) return 'bad'
	var s = num(argv[2])
	if (s >= l) return 'bad'
	var B = point('5866666666666666666666666666666666666666666666666666666666666666')
	var R = add(mul(B, s), mul([mod(-A[0]), A[1], A[2], mod(-A[3])], num(argv[3]) % l))
	return encode(R) == argv[1] ? 'ok' : 'bad'
}
EOF
}
{{- end}}

# has_webkit fails when the system's library cache has no WebKitGTK, which
# the app loads when it starts (4.1, else 4.0), with GTK, which it depends
# on. It succeeds when it cannot tell, as without ldconfig (NixOS) or a
# cache (musl).
has_webkit() {
	for ldconfig in "$(command -v ldconfig || true)" /sbin/ldconfig /usr/sbin/ldconfig; do
		[ -n "$ldconfig" ] && [ -x "$ldconfig" ] || continue
		"$ldconfig" -p >"$work/libs" 2>/dev/null || return 0
		grep -qF libc.so.6 "$work/libs" || return 0
		grep -qF -e libwebkit2gtk-4.1.so.0 -e libwebkit2gtk-4.0.so.37 "$work/libs"
		return
	done
	return 0
}

webkit_install_command() {
	distro=
	for file in /etc/os-release /usr/lib/os-release; do
		if [ -f "$file" ]; then
			distro="$(. "$file" && echo "${ID:-} ${ID_LIKE:-}")" || true
			break
		fi
	done
	for id in $distro; do
		case "$id" in
		debian | ubuntu) echo "sudo apt install libwebkit2gtk-4.1-0" && return ;;
		fedora) echo "sudo dnf install webkit2gtk4.1" && return ;;
		arch) echo "sudo pacman -S webkit2gtk-4.1" && return ;;
		opensuse* | suse) echo "sudo zypper install libwebkit2gtk-4_1-0" && return ;;
		esac
	done
	echo "the package manager of your system (the library is libwebkit2gtk-4.1.so.0)"
}

uninstall() {
	[ -d "$app_dir" ] || fail "$app_name is not installed in $app_dir"
	# Only what install.sh and the app made: a package of the system may
	# install the same names.
	for link in "$bin_dir/$name" "$bin_dir/$command_name"; do
		if ours "$link"; then
			rm -f "$link"
		fi
	done
	if [ -f "$desktop_file" ] && grep -qF "$app_dir/" "$desktop_file"; then
		rm -f "$desktop_file" "$mime_file"
	fi
	for entry in "$data_home"/applications/*.url-handler.desktop; do
		if [ -f "$entry" ] && grep -qF "$app_dir/" "$entry"; then
			rm -f "$entry"
		fi
	done
	rm -rf "$app_dir"
	update_databases
	echo "Uninstalled $app_name. Its settings and data are still in place."
}

# ours tells whether $1 is the link to the app that install.sh makes.
ours() {
	[ -L "$1" ] && [ "$(readlink "$1")" = "$app_dir/$name" ]
}

update_databases() {
	if command -v update-desktop-database >/dev/null 2>&1; then
		update-desktop-database "$data_home/applications" >/dev/null 2>&1 || true
	fi
	if [ -d "$data_home/mime/packages" ] && command -v update-mime-database >/dev/null 2>&1; then
		update-mime-database "$data_home/mime" >/dev/null 2>&1 || true
	fi
}

main "$@"
`))
