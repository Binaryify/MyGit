package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
	loc "mygit/locales"
)

// isBinary tells whether a file's text is binary, as git tells it: a NUL
// among its first 8000 bytes.
func isBinary(text string) bool {
	return strings.IndexByte(text[:min(len(text), 8000)], 0) >= 0
}

// isImage tells whether a file is a picture the diff can show as one.
func isImage(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		return true
	}
	return false
}

// picture is a side of an image's change: the picture and how big its
// file is, or nil where the side has none, as an image added.
type picture struct {
	bitmap *ui.Bitmap
	bytes  int
}

// images are a changed picture's sides, before and after.
type images struct {
	old, new *picture
	err      string
}

// side reads a picture: from a revision ("" the index's), or the working
// tree's file when worktree.
type side struct {
	rev, path string
	worktree  bool
}

func (s *side) read(ctx context.Context, repo *git.Repo) (*picture, error) {
	if s == nil {
		return nil, nil
	}
	var data []byte
	var err error
	if s.worktree {
		data, err = os.ReadFile(filepath.Join(repo.Dir, filepath.FromSlash(s.path)))
	} else {
		data, err = repo.Blob(ctx, s.rev, s.path)
	}
	if err != nil {
		return nil, err
	}
	b, err := ui.DecodeBitmap(data)
	if err != nil {
		return nil, err
	}
	return &picture{b, len(data)}, nil
}

// loadImages reads a picture's two sides for the diff shown.
func (a *app) loadImages(old, new *side) {
	gen := a.detail.gen
	if a.detail.images == nil { // else the sides shown stay until they're read again
		a.detail.images = &images{}
	}
	work(a, func(ctx context.Context) (images, error) {
		o, err := old.read(ctx, a.repo)
		if err != nil {
			return images{}, err
		}
		n, err := new.read(ctx, a.repo)
		return images{old: o, new: n}, err
	}, func(im images, err error) {
		if gen != a.detail.gen || a.detail.images == nil {
			return
		}
		if err != nil {
			im.err = err.Error()
		}
		*a.detail.images = im
	})
}

// commitSides are where a file of the commit shown was before and after:
// its parent's (the one chosen, of a merge) and its own, or a comparison's
// two revisions, the working tree when the second is empty.
func (a *app) commitSides(oldName, newName string) (old, new *side) {
	c := a.detail.commit
	from, to := "", c.Hash
	switch {
	case comparing(c):
		from, to, _ = strings.Cut(c.Hash, "..")
	case len(c.Parents) > 0:
		from = c.Parents[min(a.detail.parent, len(c.Parents)-1)]
	default:
		from = c.Hash + "^" // a stash's, or none for a root commit
	}
	if oldName != "" {
		old = &side{rev: from, path: oldName}
	}
	if newName != "" {
		new = &side{rev: to, path: newName, worktree: to == ""}
	}
	return old, new
}

// changeSides are where a change's picture was and is: HEAD's and the
// index's when staged, else the index's and the working tree's.
func changeSides(c git.Change, staged bool) (old, new *side) {
	oldPath := c.Path
	if c.OldPath != "" {
		oldPath = c.OldPath
	}
	code := c.Work
	if staged {
		code = c.Staged
	}
	if staged {
		old, new = &side{rev: "HEAD", path: oldPath}, &side{rev: "", path: c.Path}
	} else {
		old, new = &side{rev: "", path: c.Path}, &side{path: c.Path, worktree: true}
	}
	switch code {
	case 'A', '?':
		old = nil
	case 'D':
		new = nil
	}
	return old, new
}

// imageView shows a picture's change: its sides side by side, each fit
// in half the room.
func (a *app) imageView(c *ui.Context, im *images) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Padding(16).Children(func() {
		switch {
		case im.err != "":
			ui.Text(c, loc.ImageError(im.err)).TextColor(t.TextMuted).Wrap()
		case im.old == nil && im.new == nil:
			ui.Spinner(c).Size(18, 18)
		default:
			ui.Row(c).Gap(24).AlignItems(ui.End).Children(func() {
				if im.old != nil {
					pictureView(c, loc.ImageBefore(), im.old, t.Danger)
				}
				if im.new != nil {
					pictureView(c, loc.ImageAfter(), im.new, t.Success)
				}
			})
		}
	})
}

// pictureView is a side of a picture's change: its label, the picture in
// a frame of the side's color, and its size.
func pictureView(c *ui.Context, label string, p *picture, color ui.Color) {
	t := c.Theme()
	w, h := p.bitmap.Size()
	// At most 420 by 360, as a picture of a screen with a high density
	// shows at half its pixels.
	fw, fh := float32(w), float32(h)
	if s := min(420/fw, 360/fh, 1); s < 1 {
		fw, fh = fw*s, fh*s
	}
	ui.Column(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).Bold().TextColor(color)
		ui.Box(c).Padding(6).Radius(8).BorderWidth(1).BorderColor(color.Alpha(0.5)).Background(t.Surface).Children(func() {
			ui.Image(c, p.bitmap).Size(fw, fh)
		})
		ui.Text(c, fmt.Sprintf("%d × %d · %s", w, h, byteSize(p.bytes))).FontSize(11).TextColor(t.TextMuted)
	})
}

// byteSize says a file's size as Finder does.
func byteSize(n int) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d B", n)
	case n < 1000*1000:
		return fmt.Sprintf("%.1f KB", float64(n)/1000)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/1000/1000)
}
