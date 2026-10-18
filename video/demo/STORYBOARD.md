---
format: 1920x1080
duration: 40s
message: "MyGit: a light, native git client — the IDE's power in a sixth of the memory"
arc: Brand → Open → History → Stage → Branches → Conflicts → Proof → Outro
audience: developers who use git daily and find IDE-based git clients heavy
mode: collaborative
languages: en, zh-Hans
---

## Frame 1: Brand open (0-4s)

- act: Hook
- type: branding
- duration: 4s
- transition_in: cut
- blueprint: logo-assemble-lockup
- status: outline
- src: compositions/frames/01-brand.html
- scene: The logo's two branches draw down and meet; "MyGit" and the tagline land beside it.
- caption: "A light, native git client" / "轻量的原生 git 客户端"

On a deep blue field, the blue and green branch lines of the logo DRAW downward and
meet at the white merge dot, which pops; the rounded icon body fades up around them.
The wordmark slides in to the right, the tagline under it. Sets the colors the film
keeps.

## Frame 2: Drop a folder in (4-8s)

- act: Product
- type: product_intro
- duration: 4s
- transition_in: crossfade
- blueprint: cursor-ui-demo (rules: cursor-drag, spring-pop-entrance)
- status: outline
- src: compositions/frames/02-open.html
- scene: A folder is dragged onto the welcome window; the repository opens.
- caption: "Drop a folder to open it" / "拖入文件夹即可打开"

The welcome window floats center-left. A folder icon rides the cursor in from the right,
the drop zone lights up blue, and on the drop the window springs into the repository's
window — the handoff into the next frame.

## Frame 3: The history (8-14s)

- act: Product
- type: feature_showcase
- duration: 6s
- transition_in: zoom-through
- blueprint: device-surface-showcase (rules: coordinate-target-zoom, depth-of-field-blur)
- status: outline
- src: compositions/frames/03-history.html
- scene: The main window, then a push-in on the commit graph's branches and tags.
- caption: "Every branch, with its graph" / "所有分支，一张图看清"

The whole window holds for a beat, then the camera PUSHES in on the graph's colored
lanes and the v0.2.0 tag while the rest of the window softens. A commit is chosen and
its diff fills the lower half.

## Frame 4: Stage what you mean (14-20s)

- act: Features
- type: feature_showcase
- duration: 6s
- transition_in: cut
- blueprint: cursor-ui-demo (rules: cursor-click-ripple, coordinate-target-zoom)
- status: outline
- src: compositions/frames/04-stage.html
- scene: The cursor picks lines in the diff and clicks "Stage Selected Lines".
- caption: "Stage files, changes, or single lines" / "按文件、改动块，甚至按行暂存"

Close on the diff of README.md: the cursor drags across two line numbers (they tint),
then CLICKS "Stage Selected Lines" with a ripple; the file moves up into Staged Changes.

## Frame 5: Find any branch (20-25s)

- act: Features
- type: feature_showcase
- duration: 5s
- transition_in: cut
- blueprint: cursor-ui-demo (rules: cursor-click-ripple, anchored-layout-expand)
- status: outline
- src: compositions/frames/05-branches.html
- scene: The branch button opens its popover; a search narrows the list.
- caption: "Search, switch, merge or rebase" / "搜索、切换、合并、变基"

The cursor clicks the branch button; the popover drops open; "feat" TYPES into its
search and the list narrows to the feature branches.

## Frame 6: Resolve conflicts (25-31s)

- act: Features
- type: feature_showcase
- duration: 6s
- transition_in: crossfade
- blueprint: device-surface-showcase (rules: coordinate-target-zoom, cursor-click-ripple)
- status: outline
- src: compositions/frames/06-merge.html
- scene: The three-pane merge view; "Take Both" resolves the conflict.
- caption: "Resolve conflicts side by side" / "三栏对照，解决冲突"

The merge view slides in with its orange conflict bar. The camera settles on the three
panes; the cursor clicks "Take Both" and the marked block in the middle collapses into
the merged lines.

## Frame 7: A sixth of the memory (31-35s)

- act: Proof
- type: benefit_highlight
- duration: 4s
- transition_in: cut
- blueprint: dataviz-countup (rules: stat-bars-and-fills)
- status: outline
- src: compositions/frames/07-memory.html
- scene: Two bars: MyGit 109 MB, an IntelliJ-based client 806 MB.
- caption: "109 MB vs 806 MB on 20,000 commits" / "2 万个提交：109 MB 对 806 MB"

Two horizontal bars GROW from the left: a short green one for MyGit counting up to 109 MB,
a long gray one counting to 806 MB for the IntelliJ-platform client. The label "A sixth
of the memory" stamps above.

## Frame 8: Outro (35-40s)

- act: Outro
- type: cta
- duration: 5s
- transition_in: crossfade
- blueprint: logo-assemble-lockup (end card)
- status: outline
- src: compositions/frames/08-outro.html
- scene: The logo and wordmark, the platforms and languages, the repository's address.
- caption: "macOS · Windows · Linux — English / 简体中文" · "github.com/Binaryify/MyGit"

The logo settles center, "MyGit" under it; a row of platform labels and the two languages
cascade in, then the GitHub address. Holds for the last beat of the music.
