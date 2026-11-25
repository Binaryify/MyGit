package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func TestParseSubmodules(t *testing.T) {
	out := " 03558e4d themes/nivis (heads/master)\n-1a2b3c4d libs/empty\n+5e6f7a8b libs/moved (v1.0-2-g5e6f7a8)\n 9c0d1e2f themes/nivis/deep (heads/main)\n"
	got := parseSubmodules(out)
	want := []Submodule{{"themes/nivis", true}, {"libs/empty", false}, {"libs/moved", true}, {"themes/nivis/deep", true}}
	if !slices.Equal(got, want) {
		t.Fatalf("%+v, want %+v", got, want)
	}
}

// allowFileTransport lets git clone submodules from folders, as tests
// make them; git refuses since 2.38.1 unless asked.
func allowFileTransport(t *testing.T) {
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "protocol.file.allow")
	t.Setenv("GIT_CONFIG_VALUE_0", "always")
}

func TestSubmodules(t *testing.T) {
	allowFileTransport(t)
	ctx := context.Background()
	theme := repo(t)
	write(t, theme, "style.css", "a {}\n")
	_ = theme.Stage(ctx, "style.css")
	_ = theme.Commit(ctx, "Style", false)
	blog := repo(t)
	write(t, blog, "index.md", "# Blog\n")
	_ = blog.Stage(ctx, "index.md")
	if err := blog.AddSubmodule(ctx, theme.Dir, "themes/nivis"); err != nil {
		t.Fatal(err)
	}
	if err := blog.Commit(ctx, "Add the theme", false); err != nil {
		t.Fatal(err)
	}
	subs, err := blog.Submodules(ctx)
	if err != nil || !slices.Equal(subs, []Submodule{{"themes/nivis", true}}) {
		t.Fatalf("submodules %+v, %v", subs, err)
	}
	// A file changed in it changes its folder in the superproject.
	os.WriteFile(filepath.Join(blog.Dir, "themes", "nivis", "style.css"), []byte("b {}\n"), 0o644)
	st, err := blog.Status(ctx)
	if err != nil || len(st.Changes) != 1 || !st.Changes[0].Submodule || st.Changes[0].Path != "themes/nivis" {
		t.Fatalf("status %+v, %v", st.Changes, err)
	}
	sub, err := blog.OpenSubmodule("themes/nivis")
	if err != nil || sub.Dir == blog.Dir || filepath.Base(sub.Dir) != "nivis" {
		t.Fatalf("submodule opened %v, %v", sub, err)
	}
	// Cloned with them, they are checked out.
	parent := t.TempDir()
	if err := Clone(ctx, parent, blog.Dir, "copy", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(parent, "copy", "themes", "nivis", "style.css")); err != nil {
		t.Fatalf("not cloned with its submodule: %v", err)
	}
	// Cloned without them, they are empty until updated.
	if out, err := exec.Command("git", "clone", "-q", blog.Dir, filepath.Join(parent, "bare")).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	bare, err := Open(filepath.Join(parent, "bare"))
	if err != nil {
		t.Fatal(err)
	}
	if subs, _ := bare.Submodules(ctx); !slices.Equal(subs, []Submodule{{"themes/nivis", false}}) {
		t.Fatalf("submodules before updating %+v", subs)
	}
	if err := bare.UpdateSubmodules(ctx); err != nil {
		t.Fatal(err)
	}
	if subs, _ := bare.Submodules(ctx); !slices.Equal(subs, []Submodule{{"themes/nivis", true}}) {
		t.Fatalf("submodules after updating %+v", subs)
	}
}
