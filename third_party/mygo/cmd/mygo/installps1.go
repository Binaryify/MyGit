package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"unicode/utf16"

	"github.com/egoist/mygo/internal/update"
)

// The install script of Windows apps with updates: install.ps1 downloads
// the installer of the latest version for the machine's architecture
// (installerFileName), checks its SHA-256 against SHA256SUMS of the same
// release (writeChecksums), quits the app if it runs, stopping it when it
// keeps running 10 s after its windows were asked to close, as in the
// tray, and runs the installer silently, for the user, in the folder of
// the installed version, then opens the app:
//
//	irm https://github.com/me/my-app/releases/latest/download/install.ps1 | iex
//
// It runs in Windows PowerShell 5.1 and in PowerShell 7. GitHub renames
// the spaces of uploaded files to dots, which it follows, and else finds
// the installer in the release's assets with the GitHub API.
//
// For tests, $env:<NAME>_DOWNLOAD_URL is where the files are instead and
// $env:<NAME>_NO_OPEN=1 leaves the app closed (envName).

const installPS1Name = "install.ps1"

// writeInstallPS1 writes install.ps1 into dir and returns its path.
func writeInstallPS1(c *Config, dir string) (string, error) {
	path := filepath.Join(dir, installPS1Name)
	return path, os.WriteFile(path, []byte(installPS1(c)), 0o644)
}

func installPS1(c *Config) string {
	u := c.Updates
	// The comment that names the app stays ASCII too (psQuote).
	title := strings.Join(strings.Fields(c.windowsName()), " ")
	if strings.ContainsFunc(title, func(r rune) bool { return r >= 0x7f }) {
		title = "the app"
	}
	data := map[string]string{
		"Title":      title,
		"Name":       c.windowsName(),
		"Exe":        c.executableName() + ".exe",
		"Folder":     c.installDirName(),
		"Setup":      installerPrefix(c),
		"Identifier": c.Identifier,
		"Env":        envName(c.slug()),
		"Releases":   "",
		"API":        "",
		"TagPrefix":  "",
		"Assets":     "",
		"GitHub":     "",
		"Irm":        "",
	}
	switch {
	case u.tagged():
		data["Releases"] = "https://github.com/" + u.GitHub + "/releases/download/"
		data["API"] = update.GitHubAPI + "/repos/" + u.GitHub + "/releases?per_page=100"
		data["TagPrefix"] = u.TagPrefix
		data["Assets"] = update.GitHubAPI + "/repos/" + u.GitHub + "/releases/tags/"
		data["GitHub"] = "1"
		data["Irm"] = c.updateFile(installPS1Name, false)
	case u.GitHub != "":
		data["Releases"] = c.updateFile("", true)
		data["Assets"] = update.GitHubAPI + "/repos/" + u.GitHub + "/releases/latest"
		data["GitHub"] = "1"
		data["Irm"] = c.updateFile(installPS1Name, true)
	default:
		data["Releases"] = c.updateFile("", true)
		data["Irm"] = c.updateFile(installPS1Name, true)
	}
	var b strings.Builder
	if err := installPS1Template.Execute(&b, data); err != nil {
		panic(err)
	}
	return b.String()
}

// psQuote quotes s for PowerShell, in ASCII: Windows PowerShell 5.1
// decodes a script that irm downloads without a charset, as GitHub serves
// release files, as Latin-1, and takes the typographic quotes of other
// characters for the ends of a string. Other characters are [char] codes,
// as PowerShell 5.1 has no escape for them in a string.
func psQuote(s string) string {
	var parts []string
	ascii := ""
	for _, u := range utf16.Encode([]rune(s)) {
		if u >= 0x20 && u < 0x7f {
			ascii += string(rune(u))
			continue
		}
		if ascii != "" {
			parts = append(parts, "'"+strings.ReplaceAll(ascii, "'", "''")+"'")
			ascii = ""
		}
		parts = append(parts, fmt.Sprintf("[char]0x%04X", u))
	}
	if len(parts) == 0 {
		return "'" + strings.ReplaceAll(ascii, "'", "''") + "'"
	}
	if ascii != "" {
		parts = append(parts, "'"+strings.ReplaceAll(ascii, "'", "''")+"'")
	}
	// A string first, so that + joins strings rather than adds numbers.
	return "(''" + " + " + strings.Join(parts, " + ") + ")"
}

var installPS1Template = template.Must(template.New("").Funcs(template.FuncMap{"q": psQuote}).Parse(`# Installs {{.Title}} for the current user: the installer of the latest version
# for this PC, its SHA-256 checked against SHA256SUMS of its release, run
# silently. Made by mygo build.
#
#   irm {{.Irm}} | iex

& {
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$appName = {{q .Name}}
$exeName = {{q .Exe}}
$setupPrefix = {{q .Setup}}
$identifier = {{q .Identifier}}
$defaultFolder = Join-Path $env:LOCALAPPDATA ('Programs\' + {{q .Folder}})
# Where the files of the latest version are: in the release of the newest
# tag with the prefix when the repository's releases (api) hold others too.
$releases = {{q .Releases}}
$api = {{q .API}}
$tagPrefix = {{q .TagPrefix}}
# GitHub renames the spaces of uploaded files to dots; its API lists them.
$github = {{if .GitHub}}$true{{else}}$false{{end}}
$assetsApi = {{q .Assets}}
# For tests only: where the files are instead, and leaving the app closed.
$downloadUrl = $env:{{.Env}}_DOWNLOAD_URL
$noOpen = $env:{{.Env}}_NO_OPEN
if ($downloadUrl) {
	$releases = $downloadUrl.TrimEnd('/') + '/'
	$api = ''
	$github = $false
}

function Get-Arch {
	$arch = $env:PROCESSOR_ARCHITEW6432
	if (-not $arch) { $arch = $env:PROCESSOR_ARCHITECTURE }
	if ($arch -eq 'AMD64') {
		# An x64 PowerShell on an Arm PC.
		try {
			if ((Get-CimInstance Win32_Processor -ErrorAction Stop | Select-Object -First 1).Architecture -eq 12) { $arch = 'ARM64' }
		} catch {}
	}
	switch ($arch) {
		'ARM64' { return 'arm64' }
		'AMD64' { return 'amd64' }
		default { throw "$appName is not made for $arch" }
	}
}

function Get-File([string]$url, [string]$path) {
	Invoke-WebRequest -UseBasicParsing -Uri $url -OutFile $path
}

# Get-LatestTag returns the tag of the newest release tagged $tagPrefix that
# is neither a draft nor a prerelease.
function Get-LatestTag {
	for ($page = 1; $page -le 10; $page++) {
		# Windows PowerShell 5.1 outputs the JSON array as one object, which
		# ForEach-Object enumerates.
		$list = @(Invoke-RestMethod -UseBasicParsing -Uri "$api&page=$page" | ForEach-Object { $_ })
		foreach ($r in $list) {
			if (-not $r.draft -and -not $r.prerelease -and $r.tag_name.StartsWith($tagPrefix)) { return $r.tag_name }
		}
		if ($list.Count -lt 100) { break }
	}
	throw "no release of $appName is published"
}

# Get-AppProcess returns the processes running from the folder. It finds
# them with CIM, which reads every path at once: the Path of each process
# from Get-Process takes seconds in all on Windows PowerShell 5.1.
function Get-AppProcess([string]$folder) {
	$dir = $folder.TrimEnd('\') + '\'
	Get-CimInstance Win32_Process -Property ProcessId, ExecutablePath -ErrorAction SilentlyContinue | Where-Object {
		$_.ExecutablePath -and $_.ProcessId -ne $PID -and $_.ExecutablePath.StartsWith($dir, [StringComparison]::OrdinalIgnoreCase)
	} | ForEach-Object { Get-Process -Id $_.ProcessId -ErrorAction SilentlyContinue }
}

# Wait-Exit waits up to the seconds for the processes to exit.
function Wait-Exit($processes, [int]$seconds) {
	$deadline = (Get-Date).AddSeconds($seconds)
	foreach ($p in $processes) {
		$left = [int]($deadline - (Get-Date)).TotalMilliseconds
		if ($left -lt 0) { $left = 0 }
		try { [void]$p.WaitForExit($left) } catch {}
	}
}

function Install-App {
	try {
		[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
	} catch {}
	$arch = Get-Arch
	$base = $releases
	$assets = $assetsApi
	if ($api) {
		$tag = Get-LatestTag
		$base = $releases + [uri]::EscapeDataString($tag) + '/'
		$assets = $assetsApi + [uri]::EscapeDataString($tag)
	}
	$work = Join-Path ([IO.Path]::GetTempPath()) ([IO.Path]::GetRandomFileName())
	New-Item -ItemType Directory -Path $work | Out-Null
	try {
		$sumsFile = Join-Path $work 'SHA256SUMS'
		Get-File ($base + 'SHA256SUMS') $sumsFile
		$sum = $null
		$name = $null
		$pattern = '^([0-9a-fA-F]{64}) [ *](' + [regex]::Escape($setupPrefix) + '(\S+)-' + $arch + '\.exe)$'
		foreach ($line in [IO.File]::ReadAllLines($sumsFile)) {
			if ($line.TrimEnd() -match $pattern) {
				$sum = $Matches[1]
				$name = $Matches[2]
				$version = $Matches[3]
				break
			}
		}
		if (-not $name) { throw "the release has no installer of $appName for $arch" }

		Write-Host "Downloading $appName $version for $arch"
		$setup = Join-Path $work ($name -replace ' ', '.')
		if ($github) {
			try {
				Get-File ($base + ($name -replace ' ', '.')) $setup
			} catch {
				# Not the name GitHub gave it: the release's assets name it.
				$key = $name -replace '[^0-9A-Za-z]', ''
				$asset = (Invoke-RestMethod -UseBasicParsing -Uri $assets).assets |
					Where-Object { ($_.name -replace '[^0-9A-Za-z]', '') -eq $key } | Select-Object -First 1
				if (-not $asset) { throw }
				Get-File $asset.browser_download_url $setup
			}
		} else {
			Get-File ($base + [uri]::EscapeDataString($name)) $setup
		}
		$actual = (Get-FileHash -Algorithm SHA256 -Path $setup).Hash
		if ($actual -ne $sum.ToUpperInvariant()) {
			throw "$name does not have the SHA-256 of SHA256SUMS: not installing it"
		}

		# The installed version's folder, which a silent install would
		# otherwise leave for the default one.
		$folder = $defaultFolder
		$uninstallKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\' + $identifier
		$location = (Get-ItemProperty -Path $uninstallKey -ErrorAction SilentlyContinue).InstallLocation
		if ($location) { $folder = $location.TrimEnd('\') }

		# Quit the app: ask its windows to close and, after 10 s, stop the
		# processes still running from its folder, as an app in the tray
		# keeps running without windows.
		$running = @(Get-AppProcess $folder)
		if ($running.Count -gt 0) {
			Write-Host "Quitting $appName"
			foreach ($p in $running) { try { [void]$p.CloseMainWindow() } catch {} }
			Wait-Exit $running 10
			$running = @(Get-AppProcess $folder)
			if ($running.Count -gt 0) {
				$running | Stop-Process -Force -ErrorAction SilentlyContinue
				Wait-Exit $running 10
				if (@(Get-AppProcess $folder).Count -gt 0) { throw "$appName is still running: quit it and run this again" }
			}
		}

		Write-Host "Installing $appName in $folder"
		# /D, last, takes the rest of the command line, unquoted.
		$process = Start-Process -FilePath $setup -ArgumentList "/S /D=$folder" -Wait -PassThru
		if ($process.ExitCode -ne 0) { throw "the installer of $appName failed with exit code $($process.ExitCode)" }
		$exe = Join-Path $folder $exeName
		if (-not (Test-Path -LiteralPath $exe)) { throw "the installer did not install $exe" }
		if (-not $noOpen) { Start-Process -FilePath $exe }
		Write-Host "Installed $appName $version in $folder"
	} finally {
		Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
	}
}

try {
	Install-App
} catch {
	Write-Host "install.ps1: $_" -ForegroundColor Red
}
}
`))
