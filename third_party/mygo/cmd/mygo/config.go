package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/egoist/mygo/i18n"
)

// The configuration of a project is in one of these files at its root.
const (
	jsonConfig = "mygo.json"
	tsConfig   = "mygo.config.ts"
)

// Config is read from mygo.json or mygo.config.ts in the project root.
// Every field is optional.
type Config struct {
	// Name is the display name of the app (default: directory name).
	Name string `json:"name"`
	// Slug names the app's files where a name must be plain ASCII: the disk
	// images of macOS, the installers of Windows, the Linux executable, its
	// desktop entry, icon and Debian package, the update archives and
	// deltas of every platform, the variables of the install scripts, and
	// the MIME types the app defines. Lower-case
	// letters, digits and hyphens (default: the name as slugify makes it,
	// as my-app of "My App" and yun-yin-le of "云音乐").
	Slug string `json:"slug"`
	// Identifier is the reverse-DNS bundle identifier.
	Identifier string `json:"identifier"`
	Version    string `json:"version"`
	Copyright  string `json:"copyright"`
	// Publisher is who makes the app, which Windows lists with the
	// installed app (default: the copyright notice without its "©" and
	// years, as "Example Inc." of "Copyright © 2026 Example Inc.").
	Publisher string `json:"publisher"`
	// Icon is a square PNG, ideally 1024x1024 (default:
	// resources/icon.png when it exists). Beside it, the same name with
	// -dev, as icon-dev.png, is the development app's, when it exists.
	Icon string `json:"icon"`
	// Main is the Go package of the app (default ".").
	Main string `json:"main"`
	// Out is where builds are written (default "dist").
	Out string `json:"out"`
	// Bindings is the path of the generated TypeScript client (default:
	// frontend/src/mygo.ts when frontend/package.json depends on
	// mygo-runtime, src/mygo.ts when package.json at the root does, else
	// mygo.ts when the configuration has a frontend, and no client for an
	// app without one, such as an app of native UI).
	Bindings string `json:"bindings"`

	// DevURL is what the app loads during `mygo dev` in place of its built
	// frontend, usually the dev server that DevCommand starts, e.g.
	// "http://localhost:5173". Windows load it with relative URLs such as
	// "/" (see mygo.WindowOptions.URL).
	DevURL string `json:"devUrl"`
	// DevCommand runs in the project directory while `mygo dev` runs, e.g.
	// "bun run --cwd frontend dev". mygo dev launches the app once DevURL
	// answers.
	DevCommand string `json:"devCommand"`
	// BuildCommand builds the frontend before `mygo build` compiles the
	// app, e.g. "bun run --cwd frontend build". It runs in the project
	// directory.
	BuildCommand string `json:"buildCommand"`
	// FrontendDist is the directory of the built frontend. `mygo build`
	// embeds it into the app, which serves it at mygo://localhost/. During
	// `mygo dev` without DevURL, it is served from disk.
	FrontendDist string `json:"frontendDist"`

	// Resources lists extra files and directories to ship with the app of
	// every platform. Each is copied under its base name into the app's
	// resource directory (mygo.PathResources), next to the contents of the
	// project's resources directory, which are always included; its
	// directories named after a platform, such as resources/linux-amd64,
	// hold the files of one platform.
	Resources []string `json:"resources"`

	// URLSchemes are the custom URL schemes (deep links) of the app, e.g.
	// "myapp" for myapp://…, whose URLs reach mygo.App.OnOpenURL. macOS
	// registers them with the app bundle, Linux with the desktop entry
	// `mygo build` writes; apps register them at run time with
	// mygo.App.RegisterURLScheme where there is no installer.
	URLSchemes []string `json:"urlSchemes"`
	// FileAssociations are the types of files the app opens, which reach
	// mygo.App.OnOpenFile.
	FileAssociations []FileAssociation `json:"fileAssociations"`
	// I18n configures the catalogs of the app's messages, which make the
	// languages it ships. Without it, the app ships English only.
	I18n *I18n `json:"i18n"`
	// Updates configures signed updates, which mygo.Updater installs.
	Updates *Updates `json:"updates"`
	MacOS   MacOS    `json:"macos"`
	Windows Windows  `json:"windows"`
	Linux   Linux    `json:"linux"`

	root string
	file string // the configuration file, "" without one
	// langs are the languages the app ships, which languages reads once.
	langs     []appLanguage
	langsErr  error
	langsRead bool
}

// copyrightNoise is what publisher strips off a copyright notice: the
// words and signs of copyright, years and ranges of years, and "All rights
// reserved".
var copyrightNoise = regexp.MustCompile(`(?i)\bcopyright\b|©|\(c\)|\b\d{4}\b(\s*[-–]\s*(\d{4}\b|present\b))?|\ball rights reserved\b\.?`)

// publisher returns Publisher, or else the copyright notice without its
// "©" and years, or "".
func (c *Config) publisher() string {
	if p := strings.TrimSpace(c.Publisher); p != "" {
		return p
	}
	p := copyrightNoise.ReplaceAllString(c.Copyright, " ")
	return strings.Trim(strings.Join(strings.Fields(p), " "), " ,;:-–")
}

// configName names the configuration file in messages.
func (c *Config) configName() string {
	if c.file == "" {
		return jsonConfig
	}
	return filepath.Base(c.file)
}

// FileAssociation is a type of file the app opens: the system lists the app
// to open them, and opening one starts the app with it.
type FileAssociation struct {
	// Ext lists the file name extensions, without dots, e.g. ["md"].
	Ext []string `json:"ext"`
	// Name describes the files, e.g. "Markdown Document".
	Name string `json:"name"`
	// MimeType of the files. Linux identifies files by it; the app defines
	// an unregistered one (x-), and without one
	// application/x-<name>-<first extension>.
	MimeType string `json:"mimeType"`
	// Role is "Editor" (default) or "Viewer" (macOS).
	Role string `json:"role"`
}

// mimeType returns the MIME type of an association, and whether the app
// defines it: an unregistered type (x-) is the app's, as no system knows
// it.
func (c *Config) mimeType(fa FileAssociation) (string, bool) {
	if fa.MimeType != "" {
		_, sub, _ := strings.Cut(fa.MimeType, "/")
		return fa.MimeType, strings.HasPrefix(sub, "x-")
	}
	return "application/x-" + c.slug() + "-" + strings.ToLower(fa.Ext[0]), true
}

// I18n configures the catalogs of the app's messages: a JSON file per
// language in Dir, named after its BCP 47 tag ("de.json", "zh-Hant.json"),
// whose messages `mygo generate` writes typed Go and TypeScript functions
// for. The languages the app ships are those of the files.
type I18n struct {
	// Dir is the directory of the catalogs (default "locales"), a Go
	// package of its own, where the Go functions are written.
	Dir string `json:"dir"`
	// Source is the language the app is written in, whose catalog has
	// every message, and which the others fall back to (default "en").
	Source string `json:"source"`
}

// MacOS configures macOS packaging.
type MacOS struct {
	// Icon is the app's icon on macOS as an asset catalog: an Assets.car
	// compiled with actool, whose app icon is named AppIcon unless
	// infoPlist's CFBundleIconName says otherwise, or an Icon Composer .icon,
	// which builds compile on a Mac with Xcode 26 or later and skip
	// elsewhere. The .icns from icon remains the fallback. Or it is an .icns
	// in place of the one rendered from icon, for an icon drawn per size.
	Icon string `json:"icon"`
	// MinimumSystemVersion is the oldest macOS version supported (default
	// "12.0").
	MinimumSystemVersion string `json:"minimumSystemVersion"`
	// SigningIdentity signs the app and the disk image, e.g.
	// "Developer ID Application: Jane Doe (TEAMID)". The default "-" signs
	// ad hoc: the app runs on the Mac that built it, but Gatekeeper blocks
	// it on others.
	SigningIdentity string `json:"signingIdentity"`
	// Entitlements is a plist of entitlements to sign the app with.
	Entitlements string `json:"entitlements"`
	// ProvisioningProfile is embedded as Contents/embedded.provisionprofile
	// before signing. It must authorize the app's identifier and entitlements
	// and match its signing identity; ad-hoc signing cannot use a profile.
	ProvisioningProfile string `json:"provisioningProfile"`
	// HelperEntitlements gives code among the resources entitlements of its
	// own, by its path there: executables, libraries and bundles, e.g.
	//   "bin/server": "server.entitlements.plist"
	// for a helper that needs com.apple.security.cs.allow-jit under the
	// hardened runtime. Other code keeps the entitlements it is signed with.
	HelperEntitlements map[string]string `json:"helperEntitlements"`
	// InfoPlist holds extra keys for the Info.plist of the bundle, which
	// replace MyGo's own, e.g. usage descriptions for apps whose pages use
	// the camera or the microphone:
	//   "NSCameraUsageDescription": "Scan documents with the camera."
	InfoPlist map[string]any `json:"infoPlist"`
	// DMGTitle is the volume name of the disk image (default: name).
	DMGTitle string `json:"dmgTitle"`
	// Notarize submits production disk images to Apple's notary service and
	// staples the ticket. It needs a Developer ID signing identity.
	Notarize *Notarize `json:"notarize"`
}

// Notarize holds notarytool credentials stored in the keychain with
// `xcrun notarytool store-credentials <profile>`.
type Notarize struct {
	KeychainProfile string `json:"keychainProfile"`
	// Keychain is the keychain holding the profile (default: the login
	// keychain).
	Keychain string `json:"keychain"`
}

func loadConfig(root string) (*Config, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if !isDir(abs) {
		return nil, fmt.Errorf("%s is not a directory", root)
	}
	c := &Config{root: abs}
	data, file, err := readConfig(abs)
	c.file = file
	if err != nil {
		return nil, fmt.Errorf("%s: %w", c.configName(), err)
	}
	if data != nil {
		if err := json.Unmarshal(data, c); err != nil {
			return nil, fmt.Errorf("%s: %w", c.configName(), err)
		}
	}
	c.applyDefaults()
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", c.configName(), err)
	}
	return c, nil
}

// readConfig returns the configuration of the project in root as JSON, and
// the file it comes from: mygo.json, or mygo.config.ts, which a JavaScript
// runtime evaluates. Without either it returns nothing.
func readConfig(root string) (data []byte, file string, err error) {
	jsonFile, tsFile := filepath.Join(root, jsonConfig), filepath.Join(root, tsConfig)
	switch hasJSON, hasTS := fileExists(jsonFile), fileExists(tsFile); {
	case hasJSON && hasTS:
		return nil, "", fmt.Errorf("%s configures the project too; keep one of them", jsonConfig)
	case hasTS:
		data, err = evalTSConfig(root, tsFile)
		return data, tsFile, err
	case hasJSON:
		data, err = os.ReadFile(jsonFile)
		return data, jsonFile, err
	}
	return nil, "", nil
}

func (c *Config) validate() error {
	if n := c.MacOS.Notarize; n != nil && n.KeychainProfile == "" {
		return errors.New("macos.notarize needs a keychainProfile (see xcrun notarytool store-credentials)")
	}
	for i, fa := range c.FileAssociations {
		if len(fa.Ext) == 0 {
			return fmt.Errorf("fileAssociations[%d] has no ext", i)
		}
		for _, ext := range fa.Ext {
			if !extRe.MatchString(ext) {
				return fmt.Errorf("fileAssociations[%d]: %q is not a file name extension (without the dot)", i, ext)
			}
		}
		if fa.Role != "" && fa.Role != "Editor" && fa.Role != "Viewer" {
			return fmt.Errorf("fileAssociations[%d].role is Editor or Viewer", i)
		}
	}
	if ext := strings.ToLower(filepath.Ext(c.MacOS.Icon)); c.MacOS.Icon != "" && ext != ".car" && ext != ".icon" && ext != ".icns" {
		return fmt.Errorf("macos.icon %s is an Assets.car, an Icon Composer .icon or an .icns", c.MacOS.Icon)
	}
	for name, file := range c.MacOS.HelperEntitlements {
		if name == "" || file == "" {
			return fmt.Errorf("macos.helperEntitlements maps the path of code among the resources to a property list of entitlements, not %q to %q", name, file)
		}
	}
	if c.Windows.Certificate != "" && c.Windows.SignCommand != "" {
		return errors.New("windows.certificate and windows.signCommand exclude each other")
	}
	if strings.ContainsFunc(c.Windows.DisplayName, unicode.IsControl) {
		return fmt.Errorf("windows.displayName %q cannot hold control characters, such as line breaks", c.Windows.DisplayName)
	}
	if n := c.Windows.InstallDirName; n != "" {
		if err := checkFolderName(n); err != nil {
			return fmt.Errorf("windows.installDirName %q is the name of the app's folder, which %w", n, err)
		}
	}
	if c.Slug != "" && !slugRe.MatchString(c.Slug) {
		return fmt.Errorf("slug %q is not a slug (lower-case letters and digits, with single hyphens between them)", c.Slug)
	}
	if cmd := c.Linux.Command; cmd != "" && !commandRe.MatchString(cmd) {
		return fmt.Errorf("linux.command %q is not a command name (a letter or digit, then letters, digits, ., _, + or -)", cmd)
	}
	if c.Updates != nil {
		if err := c.Updates.validate(); err != nil {
			return err
		}
	}
	for _, scheme := range c.URLSchemes {
		if !schemeRe.MatchString(scheme) {
			return fmt.Errorf("urlSchemes: %q is not a URL scheme (a letter, then letters, digits, +, - or .)", scheme)
		}
	}
	for _, v := range []string{c.Name, c.Version, c.Identifier} {
		if strings.Contains(v, "'") && strings.Contains(v, `"`) {
			return fmt.Errorf("%q cannot contain both kinds of quotes", v)
		}
	}
	if c.I18n != nil {
		if src := i18n.Normalize(c.I18n.Source); src != c.I18n.Source || i18n.IsPseudo(src) {
			return fmt.Errorf("i18n.source %q is not a BCP 47 tag in its canonical form, as en, de-CH or zh-Hans", c.I18n.Source)
		}
		if filepath.IsAbs(c.I18n.Dir) || filepath.Clean(c.I18n.Dir) == "." || strings.HasPrefix(filepath.Clean(c.I18n.Dir), "..") {
			return fmt.Errorf("i18n.dir %q is not a directory of the project: the catalogs are a Go package of their own, as locales", c.I18n.Dir)
		}
	}
	if c.DevURL != "" {
		if u, err := url.Parse(c.DevURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("devUrl %q is not an http(s) URL", c.DevURL)
		}
	}
	return nil
}

var (
	nonIdent = regexp.MustCompile(`[^a-zA-Z0-9]+`)
	slugRe   = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	schemeRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*$`)
	extRe    = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_+-]*$`)
)

func (c *Config) applyDefaults() {
	if c.Name == "" {
		c.Name = filepath.Base(c.root)
	}
	if c.Identifier == "" {
		slug := strings.ToLower(nonIdent.ReplaceAllString(c.Name, ""))
		if slug == "" {
			slug = "app"
		}
		c.Identifier = "com.mygo." + slug
	}
	if c.Version == "" {
		c.Version = "0.1.0"
	}
	if c.Main == "" {
		c.Main = "."
	}
	if c.Out == "" {
		c.Out = "dist"
	}
	if c.Icon == "" && fileExists(filepath.Join(c.root, resourcesDir, "icon.png")) {
		c.Icon = filepath.Join(resourcesDir, "icon.png")
	}
	if c.MacOS.MinimumSystemVersion == "" {
		c.MacOS.MinimumSystemVersion = "12.0"
	}
	if c.MacOS.SigningIdentity == "" {
		c.MacOS.SigningIdentity = "-"
	}
	if c.MacOS.DMGTitle == "" {
		c.MacOS.DMGTitle = c.Name
	}
	if c.Linux.Maintainer == "" {
		c.Linux.Maintainer = packageAuthor(c.root)
	}
	if c.Linux.Maintainer == "" {
		c.Linux.Maintainer = c.Name
	}
	if c.I18n != nil {
		if c.I18n.Dir == "" {
			c.I18n.Dir = "locales"
		}
		if c.I18n.Source == "" {
			c.I18n.Source = "en"
		}
	}
	frontendOptions := c.DevURL != "" || c.DevCommand != "" || c.BuildCommand != "" || c.FrontendDist != ""
	// The client imports mygo-runtime: a package.json without it is not
	// the app's frontend, as that of an app of native UI that runs the
	// CLI from npm, or a workspace root that holds the app's website.
	switch {
	case c.Bindings != "":
	case usesRuntime(filepath.Join(c.root, "frontend")):
		c.Bindings = filepath.Join("frontend", "src", "mygo.ts")
	case usesRuntime(c.root):
		c.Bindings = filepath.Join("src", "mygo.ts")
	case frontendOptions:
		c.Bindings = "mygo.ts"
	}
}

// usesRuntime reports whether the package.json in dir depends on
// mygo-runtime.
func usesRuntime(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return false
	}
	var pkg struct {
		Dependencies    map[string]json.RawMessage `json:"dependencies"`
		DevDependencies map[string]json.RawMessage `json:"devDependencies"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return false
	}
	_, dep := pkg.Dependencies["mygo-runtime"]
	_, dev := pkg.DevDependencies["mygo-runtime"]
	return dep || dev
}

// packageAuthor returns the author in the package.json of root, which npm
// writes as "Name <email> (url)" or {"name", "email", "url"}, as
// "Name <email>", or "" without one.
func packageAuthor(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return ""
	}
	var pkg struct {
		Author json.RawMessage `json:"author"`
	}
	if json.Unmarshal(data, &pkg) != nil || pkg.Author == nil {
		return ""
	}
	var name, email string
	var person struct{ Name, Email string }
	if err := json.Unmarshal(pkg.Author, &name); err == nil {
		name, _, _ = strings.Cut(name, "(")
		name, email, _ = strings.Cut(name, "<")
		email, _, _ = strings.Cut(email, ">")
	} else if err := json.Unmarshal(pkg.Author, &person); err == nil {
		name, email = person.Name, person.Email
	}
	name, email = strings.Join(strings.Fields(name), " "), strings.TrimSpace(email)
	switch {
	case name == "":
		return ""
	case email == "":
		return name
	}
	return name + " <" + email + ">"
}

func (c *Config) path(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(c.root, p)
}

// executableName is a file system friendly version of Name.
func (c *Config) executableName() string {
	if name := fsName(c.Name); name != "" {
		return name
	}
	return "app"
}

// windowsName returns the name Windows shows the app by: windows.displayName,
// or else Name.
func (c *Config) windowsName() string {
	if n := strings.TrimSpace(c.Windows.DisplayName); n != "" {
		return n
	}
	return c.Name
}

// shortcutName is the file name of the app's shortcuts on Windows, without
// .lnk: windowsName made a file name, or the executable's name without one.
func (c *Config) shortcutName() string {
	if name := fsName(c.windowsName()); name != "" {
		return name
	}
	return c.executableName()
}

// installDirName returns the name of the app's install folder on Windows:
// windows.installDirName, or else Name made a file name.
func (c *Config) installDirName() string {
	if n := strings.TrimSpace(c.Windows.InstallDirName); n != "" {
		return n
	}
	return fsName(c.Name)
}

// reservedFileName matches the names Windows keeps for devices, which no
// file or folder can have, with an extension or without.
var reservedFileName = regexp.MustCompile(`(?i)^(con|prn|aux|nul|com[0-9¹²³]|lpt[0-9¹²³])(\..*)?$`)

// checkFolderName reports why name, trimmed, cannot name a folder on
// Windows: it is empty, "." or "..", holds a path separator or another
// character file names cannot hold, ends with a dot, which Windows drops,
// or is the name of a device.
func checkFolderName(name string) error {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return errors.New("is empty")
	case strings.ContainsFunc(name, func(r rune) bool { return unicode.IsControl(r) || strings.ContainsRune(`/\:*?"<>|`, r) }):
		return errors.New(`cannot hold control characters, path separators or :*?"<>|`)
	case strings.HasSuffix(name, "."):
		return errors.New("cannot be . or .., or end with a dot")
	case reservedFileName.MatchString(name):
		return errors.New("is the name of a device on Windows")
	}
	return nil
}

// slug returns Slug, or else Name as slugify makes it.
func (c *Config) slug() string {
	if c.Slug != "" {
		return c.Slug
	}
	return slugify(c.executableName())
}

// fsName makes s usable as a file name on every platform.
func fsName(s string) string {
	return strings.Map(func(r rune) rune {
		if r < ' ' || strings.ContainsRune(`/\:*?"<>|`, r) {
			return '-'
		}
		return r
	}, strings.TrimSpace(s))
}

func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

// devIcon returns the development app's icon, the icon's name with -dev
// before its extension (icon.png, icon-dev.png), when it exists, else "".
func (c *Config) devIcon() string {
	if c.Icon == "" {
		return ""
	}
	ext := filepath.Ext(c.Icon)
	dev := strings.TrimSuffix(c.Icon, ext) + "-dev" + ext
	if !fileExists(c.path(dev)) {
		return ""
	}
	return dev
}
