---
format: 1920x1080 (index.html), 1080x1920 (../vertical/index.html)
duration: 42s
message: "See what your agent changed: stage what's right, hand the rest back"
arc: Brand → Agent at work → See → Keep → Send back → The IDE you know → Proof → Outro
audience: developers who code with agents and find IDEs heavy
mode: collaborative
languages: en, zh-Hans
---

Both cuts share film.js: the words, the points in the screenshots, and the
timeline. Each cut lays out the same elements and frames each beat with its
own camera ([x, y, zoom] of the screenshots).

## Frame 1: Brand open (0-3.6s)
- blueprint: logo-assemble-lockup
- caption: "See what your agent changed" / "看清 agent 改了什么"

## Frame 2: The agent at work (3.5-11s)
- blueprint: device-surface-showcase (rules: cursor-ui-demo)
- The agent's terminal lists its edits; the window refreshes with them, the
  change list growing from none to one to three (clean → one → three).
- caption: "Changes show up as the agent works" / "agent 改着，改动就出现"

## Frame 3: See (11-17s)
- rules: cursor-click-ripple, coordinate-target-zoom
- api.ts opens side by side; the camera pushes into the retry loop. The
  lockfile opens folded away.
- caption: "Every change, side by side" / "看：每处改动，并排看清"

## Frame 4: Keep (17-22s)
- rules: cursor-click-ripple
- The lockfile and the test are staged with their + buttons; Staged Changes lights.
- caption: "Stage what's right" / "留：没问题的，先暂存"

## Frame 5: Send back (22-30s)
- rules: coordinate-target-zoom, cursor-click-ripple
- A comment on lines 10-11; "1 Comment" → "Copy as Markdown" → the copied
  bar; the Markdown pasted into the agent's terminal, and the agent edits again.
- caption: "Hand the rest back to the agent" / "退：其余的，交回给 agent"

## Frame 6: The IDE you know (30-34s)
- The graph, the branch search and the three-pane merge in quick cuts.
- caption: "The IDE you know, without the IDE" / "熟悉的 IDE 体验，不用打开 IDE"

## Frame 7: A fraction of the memory (34-38s)
- blueprint: dataviz-countup (rules: stat-bars-and-fills)
- MyGit 109 MB, VS Code 1,000 MB, IntelliJ IDEA 2,300 MB.

## Frame 8: Outro (38-42s)
- The logo, the name, the tagline, the platforms and languages, the address.
