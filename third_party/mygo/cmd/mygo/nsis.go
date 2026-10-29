package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	fetch "github.com/egoist/mygo/internal/download"
	"github.com/egoist/mygo/internal/nsis"
)

// The Windows installer is built with NSIS: a per-user install in
// %LOCALAPPDATA%\Programs, where the app can update itself, with a Start
// menu shortcut, a desktop shortcut the finish page offers, and an
// uninstaller that Settings > Apps lists. Windows rarely has NSIS
// installed, so there mygo build downloads it when it needs it, as Tauri
// and electron-builder do; elsewhere makensis must be installed.

// makensis finds the NSIS compiler: an installed one or, on Windows, the
// one an earlier build downloaded.
func makensis() string {
	if p, err := exec.LookPath("makensis"); err == nil {
		return p
	}
	if runtime.GOOS == "windows" {
		for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles"} {
			if p := filepath.Join(os.Getenv(env), "NSIS", "makensis.exe"); fileExists(p) {
				return p
			}
		}
		if dir := nsis.Dir(); dir != "" {
			return nsis.Executable(dir)
		}
	}
	return ""
}

// nsisCompiler returns the NSIS compiler, which it downloads on Windows
// when there is none, or "" on other systems without NSIS.
func nsisCompiler() (string, error) {
	if p := makensis(); p != "" || runtime.GOOS != "windows" {
		return p, nil
	}
	dir := nsis.Dir()
	if dir == "" {
		return "", errors.New("no cache directory to download NSIS into: install NSIS (makensis)")
	}
	return downloadNSIS(dir)
}

// downloadNSIS downloads nsis.Release into dir, unless it is there
// already, and returns its makensis.
func downloadNSIS(dir string) (string, error) {
	if p := nsis.Executable(dir); p != "" {
		return p, nil
	}
	v := nsis.Release.Version
	t := con.start("Downloading NSIS " + v + " for the Windows installer")
	p, err := nsis.Download(context.Background(), dir, func(url string) fetch.File { return downloadFile(t, url, "") })
	if t.end(err, "Downloaded NSIS "+v) != nil {
		return "", fmt.Errorf("downloading NSIS %s (or install it: https://nsis.sourceforge.io):\n%w", v, err)
	}
	return p, nil
}

// downloadHeaderTimeout is how long the CLI's downloads wait for a host to
// answer, so that one that takes connections and never answers gives way
// to the next copy.
var downloadHeaderTimeout = 30 * time.Second

// downloadFile is a download by the CLI of url, whose SHA-256 is sum: its
// progress shows on t, and a host that does not answer within
// downloadHeaderTimeout fails it.
func downloadFile(t *task, url, sum string) fetch.File {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = downloadHeaderTimeout
	return fetch.File{
		URL:    url,
		SHA256: sum,
		Client: &http.Client{Transport: transport, Timeout: 10 * time.Minute},
		Header: http.Header{"User-Agent": {"mygo/" + version}},
		Progress: func(total int64) io.Writer {
			return &downloadProgress{t: t, total: total}
		},
	}
}

// installerFileName is "<slug>-setup-<version>-<arch>.exe", as
// "my-app-setup-1.2.0-amd64.exe": each architecture's installer is named
// apart, so that the builds of two, uploaded to one release or bucket,
// keep both. install.ps1 finds it by installerPrefix.
func installerFileName(c *Config, goarch string) string {
	return installerPrefix(c) + fsName(c.Version) + "-" + goarch + ".exe"
}

// installerPrefix is what the names of the app's installers start with,
// before their version.
func installerPrefix(c *Config) string { return c.slug() + installerMark }

// installerMark follows the slug in the names of installers, which
// uploads publishes.
const installerMark = "-setup-"

// writeInstaller builds installerFileName in stage from the app files
// installed, built for goarch, the executable exe among them, and returns
// its path, or "" without NSIS.
func writeInstaller(c *Config, stage, work, exe, goarch string, installed []string) (string, error) {
	tool, err := nsisCompiler()
	if err != nil {
		return "", err
	}
	if tool == "" {
		warnf("Skipping the Windows installer: install NSIS (makensis)")
		return "", nil
	}
	out := filepath.Join(stage, installerFileName(c, goarch))
	var files strings.Builder
	for _, name := range installed {
		p := filepath.Join(stage, name)
		if isDir(p) {
			fmt.Fprintf(&files, "  File /r %s\n", nsisString(p))
		} else {
			fmt.Fprintf(&files, "  File %s\n", nsisString(p))
		}
	}
	icon := ""
	if c.Icon != "" {
		src, err := os.ReadFile(c.path(c.Icon))
		if err != nil {
			return "", err
		}
		ico, err := pngToICO(src)
		if err != nil {
			return "", err
		}
		path := filepath.Join(work, "installer.ico")
		if err := os.WriteFile(path, ico, 0o644); err != nil {
			return "", err
		}
		icon = "!define MUI_ICON " + nsisString(path) + "\n!define MUI_UNICON " + nsisString(path) + "\n"
	}
	signing, env, err := uninstallerSigning(c, tool)
	if err != nil {
		return "", err
	}
	script, err := installerScript(c, out, exe, files.String(), icon, signing)
	if err != nil {
		return "", err
	}
	nsi := filepath.Join(work, "installer.nsi")
	if err := writeNSIS(nsi, script); err != nil {
		return "", err
	}
	t := con.start("Creating " + filepath.Base(out))
	if signing != "" {
		t.set("signing Uninstall.exe")
	}
	cmd := exec.Command(tool, "-V2", nsi)
	cmd.Env = append(os.Environ(), env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.fail()
		return "", fmt.Errorf("makensis: %v\n%s", err, out)
	}
	t.done("Created " + filepath.Base(out))
	return out, nil
}

// installerScript returns the NSIS script of the installer out, which
// installs the files that the File commands files list, the executable exe
// among them, with the icon and signing commands of writeInstaller.
func installerScript(c *Config, out, exe, files, icon, signing string) (string, error) {
	uninstallKey := `Software\Microsoft\Windows\CurrentVersion\Uninstall\` + c.Identifier
	shortcut := nsisEscape(c.shortcutName()) + ".lnk"
	register, unregister := nsisAssociations(c, exe)
	langs, err := c.languages()
	if err != nil {
		return "", err
	}
	languages, err := nsisLanguageMacros(langs)
	if err != nil {
		return "", err
	}
	include, install, uninstall, err := nsisHooks(c)
	if err != nil {
		return "", err
	}
	return `Unicode true
ManifestDPIAware true
SetCompressor /SOLID lzma
` + nsisName(c.windowsName()) + `
OutFile ` + nsisString(out) + `
` + signing + `InstallDir "$LOCALAPPDATA\Programs\` + nsisEscape(c.installDirName()) + `"
; An installed app is updated in its folder, the one the user chose or the
; one of a version with another windows.installDirName, as install.ps1 and
; the app's own updates do.
InstallDirRegKey HKCU ` + nsisString(uninstallKey) + ` "InstallLocation"
RequestExecutionLevel user
BrandingText " "
` + icon + `!define MUI_FINISHPAGE_RUN "$INSTDIR\` + nsisEscape(exe) + `"
!define MUI_FINISHPAGE_SHOWREADME
!define MUI_FINISHPAGE_SHOWREADME_TEXT "Create a desktop shortcut"
!define MUI_FINISHPAGE_SHOWREADME_FUNCTION CreateDesktopShortcut
!include "MUI2.nsh"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
` + languages + include + `
` + nsisCloseApp(c) + `
Section
  Push "$INSTDIR"
  Call CloseApp
  SetOutPath "$INSTDIR"
` + files + `  WriteUninstaller "$INSTDIR\Uninstall.exe"
` + nsisRenamedShortcuts(c) + `  CreateShortCut "$SMPROGRAMS\` + shortcut + `" "$INSTDIR\` + nsisEscape(exe) + `"
` + nsisUninstallEntry(c, uninstallKey, exe) + register + install + `SectionEnd

; The finish page offers it, so silent installs (/S) make none.
Function CreateDesktopShortcut
  SetOutPath "$INSTDIR"
  CreateShortCut "$DESKTOP\` + shortcut + `" "$INSTDIR\` + nsisEscape(exe) + `"
FunctionEnd

; Deletes the shortcut on the stack, trying again for 5 s while it is in
; use: Explorer opens a new shortcut a few seconds after it appears, not
; sharing it for deletion, and Delete fails meanwhile. It succeeds when
; there is no shortcut.
Function un.DeleteShortcut
  Exch $0
  Push $1
  StrCpy $1 50
  retry:
    ClearErrors
    Delete $0
    IfErrors 0 done
    IntOp $1 $1 - 1
    IntCmp $1 0 done
    Sleep 100
    Goto retry
  done:
  Pop $1
  Pop $0
FunctionEnd

Section "Uninstall"
  Push "$INSTDIR"
  Call un.CloseApp
  Push "$SMPROGRAMS\` + shortcut + `"
  Call un.DeleteShortcut
  Push "$DESKTOP\` + shortcut + `"
  Call un.DeleteShortcut
  DeleteRegKey HKCU ` + nsisString(uninstallKey) + `
` + unregister + uninstall + `  ; Last, so that the app's folder is gone once the uninstall is done:
  ; the uninstaller runs from a copy of itself that nothing waits for.
  RMDir /r "$INSTDIR"
SectionEnd
`, nil
}

// nsisHooks returns the commands that let windows.installerInclude, a
// script of the app's own, add to the installer, as electron-builder's
// include does: include includes it, after MUI2.nsh and the languages, so
// that its macros can use them, and runs its customInit macro in
// .onInit; install runs its customInstall macro at the end of the install,
// once the files are in $INSTDIR, and uninstall its customUnInstall macro
// before the uninstaller removes $INSTDIR. Each macro runs only when the
// script defines it, and silent installs (/S) run them too. They are all
// "" without windows.installerInclude, which leaves the script as it was.
func nsisHooks(c *Config) (include, install, uninstall string, err error) {
	if c.Windows.InstallerInclude == "" {
		return "", "", "", nil
	}
	path := c.path(c.Windows.InstallerInclude)
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return "", "", "", fmt.Errorf("windows.installerInclude: %s is not a file (the path is relative to the project's directory)", path)
	}
	include = `!include ` + nsisString(path) + `
!ifmacrodef customInit
Function .onInit
  !insertmacro customInit
FunctionEnd
!endif
`
	install = `  !ifmacrodef customInstall
    !insertmacro customInstall
  !endif
`
	uninstall = `  !ifmacrodef customUnInstall
    !insertmacro customUnInstall
  !endif
`
	return include, install, uninstall, nil
}

// nsisName returns the Name command of the installer, which titles its
// pages: name, and the same with doubled ampersands where it has any, which
// the texts of buttons and labels need lest "&" underline the next letter.
func nsisName(name string) string {
	if !strings.Contains(name, "&") {
		return "Name " + nsisString(name)
	}
	return "Name " + nsisString(name) + " " + nsisString(strings.ReplaceAll(name, "&", "&&"))
}

// nsisRenamedShortcuts returns the installer commands that remove the
// shortcuts an earlier version made under the name when windows.displayName
// names them otherwise: the Start menu's goes, and a desktop one is made
// again under the new name, as the user chose to have one.
func nsisRenamedShortcuts(c *Config) string {
	old := c.executableName()
	if strings.EqualFold(old, c.shortcutName()) {
		return ""
	}
	old = nsisEscape(old) + ".lnk"
	return `  Delete "$SMPROGRAMS\` + old + `"
  IfFileExists "$DESKTOP\` + old + `" 0 +3
  Delete "$DESKTOP\` + old + `"
  Call CreateDesktopShortcut
`
}

// nsisUninstallEntry returns the installer commands that write the entry
// of the app in Settings > Apps, under uninstallKey.
func nsisUninstallEntry(c *Config, uninstallKey, exe string) string {
	var b strings.Builder
	str := func(name, value string) {
		fmt.Fprintf(&b, "  WriteRegStr HKCU %s %s %s\n", nsisString(uninstallKey), nsisString(name), value)
	}
	str("DisplayName", nsisString(c.windowsName()))
	str("DisplayVersion", nsisString(c.Version))
	str("DisplayIcon", `"$INSTDIR\`+nsisEscape(exe)+`"`)
	if p := c.publisher(); p != "" {
		str("Publisher", nsisString(p))
	}
	str("InstallLocation", `"$INSTDIR"`)
	str("UninstallString", `'"$INSTDIR\Uninstall.exe"'`)
	str("QuietUninstallString", `'"$INSTDIR\Uninstall.exe" /S'`)
	fmt.Fprintf(&b, "  WriteRegDWORD HKCU %s \"NoModify\" 1\n", nsisString(uninstallKey))
	fmt.Fprintf(&b, "  WriteRegDWORD HKCU %s \"NoRepair\" 1\n", nsisString(uninstallKey))
	return b.String()
}

// closeAppPS1 is the PowerShell that quits the processes of the app,
// those whose executables are in the folder $env:MYGO_APP_DIR: it asks
// their windows to close and, after 10 s, stops those still running, as an
// app in the tray or menu bar keeps running without windows. With
// $env:MYGO_APP_CHECK=1, it only tells whether any runs: exit code 2 when
// one does. Exit code 1 is for processes that did not stop. It finds the
// processes with CIM, which sees the paths of 64-bit processes from a
// 32-bit PowerShell too.
const closeAppPS1 = `$ErrorActionPreference = 'SilentlyContinue'
$dir = $env:MYGO_APP_DIR.TrimEnd('\') + '\'
# Not the installer, which starts this.
$installer = (Get-CimInstance Win32_Process -Filter "ProcessId=$PID").ParentProcessId
function Get-AppProcess {
	@(Get-CimInstance Win32_Process | Where-Object {
		$_.ExecutablePath -and $_.ProcessId -ne $PID -and $_.ProcessId -ne $installer -and $_.ExecutablePath.StartsWith($dir, [StringComparison]::OrdinalIgnoreCase)
	} | ForEach-Object { Get-Process -Id $_.ProcessId })
}
$running = Get-AppProcess
if ($running.Count -eq 0) { exit 0 }
if ($env:MYGO_APP_CHECK -eq '1') { exit 2 }
foreach ($p in $running) { [void]$p.CloseMainWindow() }
$deadline = (Get-Date).AddSeconds(10)
foreach ($p in $running) {
	$left = [int]($deadline - (Get-Date)).TotalMilliseconds
	if ($left -lt 0) { $left = 0 }
	[void]$p.WaitForExit($left)
}
$running = @(Get-AppProcess)
if ($running.Count -eq 0) { exit 0 }
$running | Stop-Process -Force
$deadline = (Get-Date).AddSeconds(10)
foreach ($p in $running) {
	$left = [int]($deadline - (Get-Date)).TotalMilliseconds
	if ($left -lt 0) { $left = 0 }
	[void]$p.WaitForExit($left)
}
if (@(Get-AppProcess).Count -gt 0) { exit 1 }
exit 0
`

// nsisCloseApp returns the installer and uninstaller functions CloseApp,
// which quit the app running from the folder on the stack before its
// files are replaced or deleted (closeAppPS1). A silent install or
// uninstall (/S), as an update runs, quits it without asking; the others
// ask first and stop when the user cancels. Windows without PowerShell go
// on as before.
func nsisCloseApp(c *Config) string {
	// NSIS strings hold 1024 characters, too few for the script on the
	// command line: the installer writes it to a file, line by line, which
	// PowerShell reads, named by the environment, as quotes would not do
	// for every path. Execution policies do not apply to a command.
	var write strings.Builder
	for _, line := range strings.SplitAfter(closeAppPS1, "\n") {
		if line != "" {
			write.WriteString("  FileWrite $2 " + nsisString(strings.TrimSuffix(line, "\n")+"\r\n") + "\n")
		}
	}
	command := `'"$1" -NoProfile -NonInteractive -ExecutionPolicy Bypass -Command "& ([scriptblock]::Create([IO.File]::ReadAllText($$env:MYGO_APP_SCRIPT)))"'`
	running := nsisString(c.windowsName() + " is running. Close it to continue?")
	stuck := nsisString(c.windowsName() + " is still running. Quit it, then try again.")
	var out strings.Builder
	for _, prefix := range []string{"", "un."} {
		out.WriteString(`; Quits the app running from the folder on the stack.
Function ` + prefix + `CloseApp
  Exch $0
  Push $1
  Push $2
  ; The PowerShell of the system's own architecture, not the 32-bit one
  ; that a 32-bit installer would otherwise start.
  StrCpy $1 "$WINDIR\Sysnative\WindowsPowerShell\v1.0\powershell.exe"
  IfFileExists $1 +2
  StrCpy $1 "$SYSDIR\WindowsPowerShell\v1.0\powershell.exe"
  IfFileExists $1 0 done
  InitPluginsDir
  FileOpen $2 "$PLUGINSDIR\close-app.ps1" w
` + write.String() + `  FileClose $2
  System::Call 'Kernel32::SetEnvironmentVariable(t "MYGO_APP_SCRIPT", t "$PLUGINSDIR\close-app.ps1")'
  System::Call 'Kernel32::SetEnvironmentVariable(t "MYGO_APP_DIR", t r0)'
  IfSilent close
  System::Call 'Kernel32::SetEnvironmentVariable(t "MYGO_APP_CHECK", t "1")'
  nsExec::Exec ` + command + `
  Pop $2
  System::Call 'Kernel32::SetEnvironmentVariable(t "MYGO_APP_CHECK", t "")'
  StrCmp $2 "2" 0 done
  MessageBox MB_OKCANCEL|MB_ICONEXCLAMATION ` + running + ` IDOK close
  Abort
  close:
  nsExec::Exec ` + command + `
  Pop $2
  StrCmp $2 "1" 0 done
  MessageBox MB_OK|MB_ICONSTOP ` + stuck + ` /SD IDOK
  SetErrorLevel 2
  Quit
  done:
  Pop $2
  Pop $1
  Pop $0
FunctionEnd

`)
	}
	return out.String()
}

// uninstallerSigning returns the NSIS command that signs the uninstaller
// of a signed app, and the environment makensis needs to run it. makensis
// writes the uninstaller to a temporary file, runs the command with it and
// puts the signed file into the installer. The command runs mygo, which
// signs the uninstaller as it signs the app (runSignUninstaller); its path
// goes through the environment, since NSIS reads "$" in the script as its
// own syntax.
func uninstallerSigning(c *Config, tool string) (script string, env []string, err error) {
	if !c.Windows.signs() {
		return "", nil, nil
	}
	if major, minor := nsisVersion(tool); major < 3 || major == 3 && minor < 8 {
		warnf("Not signing Uninstall.exe: that needs NSIS 3.08 or later, not %s", tool)
		return "", nil, nil
	}
	self, err := os.Executable()
	if err != nil {
		return "", nil, err
	}
	settings, err := json.Marshal(signSettings{Root: c.root, Windows: c.Windows})
	if err != nil {
		return "", nil, err
	}
	signer := `"%` + signerEnv + `%"`
	if runtime.GOOS != "windows" {
		signer = `"$` + signerEnv + `"`
	}
	// "= 0" stops the build when signing fails; NSIS ignores the exit code
	// otherwise.
	return "!uninstfinalize '" + signer + ` sign-uninstaller "%1"' = 0` + "\n",
		[]string{signerEnv + "=" + self, signSettingsEnv + "=" + string(settings)}, nil
}

// nsisVersion returns the version of the NSIS compiler tool, or zeros when
// it does not tell.
func nsisVersion(tool string) (major, minor int) {
	out, err := exec.Command(tool, "-VERSION").Output()
	if err == nil {
		// "v3.13", or "v3.08-3" for a distribution's build.
		fmt.Sscanf(strings.TrimPrefix(strings.TrimSpace(string(out)), "v"), "%d.%d", &major, &minor)
	}
	return major, minor
}

// nsisAssociations returns the installer commands that register, and
// unregister, the file associations and URL schemes of the app for the
// user, opening them with exe.
func nsisAssociations(c *Config, exe string) (register, unregister string) {
	var r, u strings.Builder
	open := `'"$INSTDIR\` + nsisEscape(exe) + `" "%1"'`
	prefix := strings.Map(func(r rune) rune {
		if r < 0x80 && (r == '.' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return r
		}
		return -1
	}, c.Identifier)
	for _, fa := range c.FileAssociations {
		for _, ext := range fa.Ext {
			progID := prefix + "." + strings.ToLower(ext)
			class := `Software\Classes\` + progID
			fmt.Fprintf(&r, "  WriteRegStr HKCU %s \"\" %s\n", nsisString(class), nsisString(fa.Name))
			fmt.Fprintf(&r, "  WriteRegStr HKCU %s \"\" \"$INSTDIR\\%s,0\"\n", nsisString(class+`\DefaultIcon`), nsisEscape(exe))
			fmt.Fprintf(&r, "  WriteRegStr HKCU %s \"\" %s\n", nsisString(class+`\shell\open\command`), open)
			fmt.Fprintf(&r, "  WriteRegStr HKCU %s %s \"\"\n", nsisString(`Software\Classes\.`+ext+`\OpenWithProgids`), nsisString(progID))
			fmt.Fprintf(&u, "  DeleteRegKey HKCU %s\n", nsisString(class))
			fmt.Fprintf(&u, "  DeleteRegValue HKCU %s %s\n", nsisString(`Software\Classes\.`+ext+`\OpenWithProgids`), nsisString(progID))
		}
	}
	for _, scheme := range c.URLSchemes {
		key := `Software\Classes\` + scheme
		fmt.Fprintf(&r, "  WriteRegStr HKCU %s \"\" %s\n", nsisString(key), nsisString("URL:"+c.windowsName()))
		fmt.Fprintf(&r, "  WriteRegStr HKCU %s \"URL Protocol\" \"\"\n", nsisString(key))
		fmt.Fprintf(&r, "  WriteRegStr HKCU %s \"\" %s\n", nsisString(key+`\shell\open\command`), open)
		fmt.Fprintf(&u, "  DeleteRegKey HKCU %s\n", nsisString(key))
	}
	if r.Len() > 0 {
		// Tell Explorer the associations changed.
		notify := "  System::Call 'shell32::SHChangeNotify(i 0x08000000, i 0, p 0, p 0)'\n"
		r.WriteString(notify)
		u.WriteString(notify)
	}
	return r.String(), u.String()
}

// writeNSIS writes the installer's script to path as UTF-8 with a byte order
// mark. The script starts with `Unicode true`, and NSIS reads a script
// without a BOM as ANSI, in the machine's code page, so a name or a path with
// a non-ASCII character, such as 望言字幕导出 or a folder named 中文, would
// not match the files it installs: makensis reports "no files found".
func writeNSIS(path, script string) error {
	b := append([]byte{0xEF, 0xBB, 0xBF}, script...)
	return os.WriteFile(path, b, 0o644)
}

// nsisEscape escapes text for an NSIS string in double quotes.
func nsisEscape(s string) string {
	return strings.NewReplacer(`$`, `$$`, `"`, `$\"`, "\n", `$\n`, "\r", `$\r`, "\t", `$\t`).Replace(s)
}

func nsisString(s string) string { return `"` + nsisEscape(s) + `"` }

// downloadProgress shows how much of a download arrived on t.
type downloadProgress struct {
	t     *task
	n     int64
	total int64 // or -1
}

func (p *downloadProgress) Write(b []byte) (int, error) {
	p.n += int64(len(b))
	if p.total > 0 {
		p.t.set(fmt.Sprintf("%s of %s", formatSize(p.n), formatSize(p.total)))
	} else {
		p.t.set(formatSize(p.n))
	}
	return len(b), nil
}
