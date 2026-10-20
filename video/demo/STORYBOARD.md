---
format: 1920x1080 (index.html), 1080x1920 (../vertical/index.html)
duration: 48s
message: "Check your agent's changes, fast: open it from a terminal, stage what's right, hand the rest back"
arc: Brand → Agent at work → See → Keep → Send back → The IDE you know → Over SSH → Proof → Outro
audience: developers who code with agents and find IDEs heavy
mode: collaborative
languages: en, zh-Hans
---

Both cuts share film.js: the words, the points in the screenshots, and the
timeline. Each cut lays out the same elements and frames each beat with its
own camera ([x, y, zoom] of the screenshots).

## Frame 1: Brand open (0-3.6s)
- blueprint: logo-assemble-lockup
- caption: "Check your agent's changes, fast" / "快速查看 agent 的改动"

## Frame 2: The agent at work (3.5-11s)
- blueprint: device-surface-showcase (rules: cursor-ui-demo)
- The agent's terminal lists its edits; the window refreshes with them, the
  change list growing from none to one to three (clean → one → three).
- caption: "It edits, you see it. No refresh" / "不用刷新，它改到哪你看到哪"

## Frame 3: See (11-17s)
- rules: cursor-click-ripple, coordinate-target-zoom
- api.ts opens side by side; the camera pushes into the retry loop. The
  lockfile opens folded away.
- caption: "One click, side by side" / "看：点一下，就是并排 diff"

## Frame 4: Keep (17-22s)
- rules: cursor-click-ripple
- The lockfile and the test are staged with their + buttons; Staged Changes lights.
- caption: "Stage what's right, in one click" / "留：没问题的，一键暂存"

## Frame 5: Send back (22-30s)
- rules: coordinate-target-zoom, cursor-click-ripple
- A comment on lines 10-11; "1 Comment" → "Copy as Markdown" → the copied
  bar; the Markdown pasted into the agent's terminal, and the agent edits again.
- caption: "Copy the rest back to the agent" / "退：其余的，一键交回 agent"

## Frame 6: Opens at once (30-34s)
- The graph, the branch search and the three-pane merge in quick cuts.
- eyebrow: "$ mygit ."
- caption: "Opens at once. No IDE needed" / "终端一句命令，秒开，不用等 IDE"

## Frame 6b: Over SSH (34-40s)
- rules: coordinate-target-zoom, cursor-click-ripple
- devbox's folders in the dialog that opens a repository over SSH, orbit
  among its repositories; Open on orbit, and its window, the camera pushing
  into the "SSH · devbox" badge, which lights.
- eyebrow: "$ ssh devbox"
- caption: "Repos on other machines, too" / "其他机器上的仓库，SSH 直接打开"
- sub: "Over SSH, to Linux, macOS or Windows. Only git needed there"
- Shots rendered by `go test -run DumpSSH` (MYGIT_DUMP, MYGIT_LOCALE), given
  the window's chrome of the screenshots.

## Frame 7: Fast, in a fraction of the memory (40-44s)
- blueprint: dataviz-countup (rules: stat-bars-and-fills)
- 0.13 s to read 20,000 commits, 0.24 s to open a 20 MB commit; then MyGit 109 MB, VS Code 1,000 MB, IntelliJ IDEA 2,300 MB.

## Frame 8: Outro (44-48s)
- The logo, the name, the tagline, the platforms and languages, the address.
