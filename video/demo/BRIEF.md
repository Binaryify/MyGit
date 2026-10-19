---
workflow: general-video
flow: automation
storyboard: yes
message: "MyGit: see what your agent changed — stage what's right, hand the rest back; the IDE you know, in a fraction of the memory"
destination: github-readme, site, douyin
aspect: 1920x1080 (video/demo), 1080x1920 (video/vertical)
language: en, zh-Hans
audience: developers who use git daily and find IDE-based git clients heavy
length: 42s
angle: feature walkthrough
---

## Intent

A short demo for the top of MyGit's README: what it is and the four things it does
best, shown in the real app. Two versions — English for README.md, Simplified
Chinese for README.zh-CN.md — with the app's UI and the captions in each language.

## Assets

- ../../docs/images/logo.png, ../../resources/logo.svg — the MyGit logo; opens and closes the film.
- Real app screenshots of a demo repository (orbit), in English and Chinese, taken for this film:
  the history with its graph, a commit's diff, staging lines and editing in the diff, the branch
  search popover, the three-pane conflict view, and the welcome window.

## Customizations

- Screenshots animated as a walkthrough: a cursor moving and clicking, push-ins on the detail,
  captions per beat. No screen recording.
- A light, upbeat music bed; cuts on its beat.

## Notes

- README videos play muted by default: the captions must carry the message without the music.
- The README links a poster image to the MP4 in the repository, which GitHub's file view plays.
- Keep to the app's own look: its dark UI, and the logo's blue and green.
- Claims only what is measured: 109 MB for MyGit, 1,000 MB for VS Code and 2,300 MB for IntelliJ IDEA on a 20,000-commit repository.
- The portrait cut keeps its words and the window above the bottom 340 px, where Douyin draws its own captions and buttons.
- The screenshots are of the real app on the orbit demo repository, its working tree edited step by step as an agent would.
