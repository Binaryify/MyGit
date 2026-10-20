<p align="center">
  <img src="docs/images/logo.png" width="128" height="128" alt="MyGit">
</p>

<h1 align="center">MyGit</h1>

<p align="center">
  <b>Check your agent's changes, fast</b><br>
  <code>mygit .</code> in a terminal opens it in a moment, and it refreshes as the agent works; stage what's right and commit, without waiting for an IDE.<br>
  A light, fast, modern git client: JetBrains' commit graph and history tools, VS Code's way of staging, in a fraction of the memory.
</p>

<p align="center">
  <a href="https://github.com/Binaryify/MyGit/actions/workflows/ci.yml"><img src="https://github.com/Binaryify/MyGit/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <img src="https://img.shields.io/badge/macOS-Apple%20silicon-111?logo=apple" alt="macOS">
  <img src="https://img.shields.io/badge/Windows-x64-0078D4?logo=windows" alt="Windows">
  <img src="https://img.shields.io/badge/Linux-x64-FCC624?logo=linux&logoColor=111" alt="Linux">
  <img src="https://img.shields.io/badge/built%20with-Go%20%2B%20MyGo-00ADD8?logo=go&logoColor=white" alt="Go">
</p>

<p align="center">
  <a href="https://binaryify.com/mygit/">Website</a> · <b>English</b> · <a href="README.zh-CN.md">简体中文</a>
</p>

<p align="center">
  <img src="docs/images/main-en.png" alt="MyGit showing a repository's history and a commit's diff">
</p>

<p align="center">
  <a href="site/assets/mygit-demo-en.mp4"><b>▶ Watch the demo</b></a>
</p>

## Why MyGit

With an agent writing the code, most of your time in git goes to looking at what changed. MyGit makes that quick:

1. **See** every change as it lands: the window refreshes while the agent works, each file side by side with syntax highlighting. Lockfiles, minified and generated files fold away, and changed pictures show before and after.
2. **Keep** what's right: fix a line right in the diff, stage the files, changes or lines you want, and commit.
3. **Send back** what isn't: comment on the lines to redo, copy the comments as Markdown, each with its file, lines and code, and paste them to Claude Code, Codex or a colleague.

MyGit reads the repository, not an agent's session, so it works the same with any agent, with several at once, and with your own edits.

| How you look at an agent's changes today | What MyGit does instead |
|---|---|
| The agent's own diff view | Works with every agent, and lets you stage single lines and fix the code before you commit |
| Opening an IDE | Opens in a moment, in a fraction of the memory |
| `git diff` in a terminal | Side by side, generated files folded, refreshed as the agent works |

### Light, fast and modern

MyGit feels like the IDE you know: JetBrains' commit graph and history tools, VS Code's list of changes and way of staging, ⌘K for commands and ⌘P to go to a file, light and dark. Without the IDE: it's a single native binary drawn on the GPU by [MyGo](https://github.com/egoist/mygo), no webview, no JVM. Memory once a repository of 20,000 commits is open, every process of the app counted (Apple M4):

| App | Memory |
|---|---|
| **MyGit** | **109 MB** |
| A trimmed IntelliJ-based client | 750 MB |
| VS Code, no extensions | 1,000 MB |
| VS Code, 167 extensions | 1,400 MB |
| IntelliJ IDEA 2026.2 | 2,300 MB |

On the same repository, MyGit reads the whole history and lays out its graph in **0.13 s**, opens a commit with a 20 MB patch (5,677 files) in **0.24 s**, and uses **0 %** CPU while idle.

MyGit reads what `git` prints and lets `git` itself make every change, so a repository behaves in MyGit exactly as it does in a terminal. The same app, with the same features, runs on macOS, Windows and Linux.

## Features

<table>
  <tr>
    <td width="50%"><img src="docs/images/changes-en.png" alt="Editing a change in the diff"></td>
    <td width="50%"><img src="docs/images/merge-en.png" alt="Resolving a conflict"></td>
  </tr>
  <tr>
    <td><b>Stage what you mean.</b> Stage files, changes or single lines, and edit the working tree right in the diff.</td>
    <td><b>Resolve conflicts side by side.</b> Ours, the result and theirs, a conflict at a time.</td>
  </tr>
  <tr>
    <td><img src="docs/images/branches-en.png" alt="Searching branches"></td>
    <td><img src="docs/images/welcome-en.png" alt="The welcome window"></td>
  </tr>
  <tr>
    <td><b>Find any branch.</b> Search local and remote branches, switch, merge, rebase or compare.</td>
    <td><b>Open anything.</b> Drop a folder, clone, or pick a recent repository.</td>
  </tr>
</table>

- **Reading changes** — generated files (lockfiles, minified, `linguist-generated`) fold away; mark files viewed; changed pictures show before and after; comment on lines and copy the comments as Markdown for an agent or a colleague
- **Staging** — stage, unstage or discard files, changes or selected lines; edit and save the working tree in the diff
- **Commits** — whole messages, changed files and side-by-side diffs with syntax highlighting; a merge against any parent; compare two commits (⌘-click), a branch or the working tree
- **History** — every branch, tag and remote with its graph, streamed as it loads; search by message, author or hash; filter by branch or path (file history)
- **History tools** — branch, tag, check out, cherry-pick, revert, reset (soft / mixed / hard) and interactive rebase (reorder, reword, edit, squash, fixup, drop)
- **Pull requests** — open a GitHub pull request, read and reply to its threads under their lines, resolve them, and submit a review: comment, approve or request changes
- **Conflicts** — a three-pane merge view, and continue, skip or abort a merge, rebase, cherry-pick or revert
- **Stash, blame and remotes** — stash with untracked files, blame a file line by line, manage remotes, pull (fast-forward, rebase or merge), preview a push, force with lease
- **Sign-ins** — when git needs a username, token or passphrase, the window asks
- **Over SSH** — open a repository on another computer (`host:/path/to/repo`, Linux, macOS or Windows) with your ssh config and keys; MyGit runs a small helper there, in `~/.mygit`, and needs only git on it
- **Everywhere** — a window per repository, drag folders in, `mygit .` from a terminal, custom title bars, light and dark, English and 简体中文, signed auto-updates

## Install

Download the latest release:

| Platform | Download |
|---|---|
| macOS (Apple silicon) | [MyGit-macos-arm64.dmg](https://github.com/Binaryify/MyGit/releases/latest/download/MyGit-macos-arm64.dmg) |
| Windows (x64) | [Installer](https://github.com/Binaryify/MyGit/releases/latest/download/MyGit-windows-amd64-setup.exe) · [zip](https://github.com/Binaryify/MyGit/releases/latest/download/MyGit-windows-amd64.zip) |
| Linux (x64) | [.deb](https://github.com/Binaryify/MyGit/releases/latest/download/MyGit-linux-amd64.deb), or the script below |

On Linux, this installs MyGit for you in `~/.local`, without root:

```bash
curl -fsSL https://github.com/Binaryify/MyGit/releases/latest/download/install.sh | sh
```

MyGit isn't notarized by Apple yet. If macOS says it can't be opened, move it to Applications and run:

```bash
xattr -dr com.apple.quarantine /Applications/MyGit.app
```

The Windows installer isn't code-signed yet either. If Windows says it protected your PC, choose **More info**, then **Run anyway**.

MyGit needs `git` on the `PATH`, and updates itself once installed. Intel Macs aren't supported. All releases are on the [releases page](https://github.com/Binaryify/MyGit/releases).

To build it from source instead, see [Development](#development).

## Shortcuts

| Keys | Does |
|---|---|
| ⌘↩ / Ctrl+Enter | Commit |
| ⌘F / Ctrl+F | Search the history |
| ⌘R / Ctrl+R | Refresh |
| ⌘P / Ctrl+P | Go to a file of the commit or the changes |
| ⌘K / Ctrl+K | Commands |
| ⌘S / Ctrl+S | Save the file edited in the diff |
| Space | Stage or unstage the change chosen; mark a commit's file viewed |
| Delete | Discard the change chosen |
| ⌘-click | Compare two commits |

## Development

```bash
git clone https://github.com/Binaryify/MyGit.git && cd MyGit   # needs Go 1.27 or later
go tool mygo dev      # run, reloading on changes
go test ./...         # the tests, the UI's included, need no window
go tool mygo build -platform darwin/arm64,windows/amd64,linux/amd64
```

The texts live in [`locales/`](locales): `en.json` is the source and `zh-Hans.json` its Chinese. After changing them, run `go tool mygo generate`. Translations into other languages are welcome: add `locales/<tag>.json`.

To release, describe the version in [`CHANGELOG.md`](CHANGELOG.md), whose section becomes the release notes, and push a tag such as `v0.2.0`: CI signs the updates with the `MYGO_UPDATER_PRIVATE_KEY` secret, builds the macOS disk image, the Windows installer and zip, and the Linux package, and publishes them in a release.

Over SSH: a release also builds and signs the helper for six platforms (`mygit-helper-<os>-<arch>.gz`), which the app downloads and checks the first time it reaches a computer of each. To try local changes, `go run ./internal/tools/helpers` builds them into `build/helpers`; start the app with `MYGIT_HELPER_DIR=$PWD/build/helpers`. `MYGIT_E2E_HOST=<host> MYGIT_HELPER_DIR=$PWD/build/helpers go test ./internal/remote -run E2E -v` tests against a real computer.

Benchmarks: `MYGIT_BENCH_REPO=<repo> MYGIT_BENCH_COMMIT=<commit> go test -run '^$' -bench .`

## License

[MIT](LICENSE)
