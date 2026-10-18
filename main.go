// MyGit is a light git client: the history with its graph, the working
// tree's changes to stage and commit, and the diffs of both.
//
//	mygit [dir]   open the repository dir is in, the current folder's by default
//
// A running MyGit opens the repositories later launches name, each in a
// window of its own.
package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
)

// windows are the open repositories' windows, by working tree.
var windows = map[string]*mygo.Window{}

func main() {
	mygo.App.SetName("MyGit")
	if !mygo.App.RequestSingleInstanceLock() {
		return // the running MyGit opens it
	}
	mygo.App.OnSecondInstance(func(args []string, cwd string) { openArg(args, cwd) })
	mygo.App.OnOpenFile(func(path string) { openDir(path) }) // Open Recent, the Dock menu
	mygo.App.OnActivate(func(visible bool) {
		if !visible {
			go pickAndOpen()
		}
	})
	mygo.App.WhenReady(func() {
		mygo.App.SetMenu(appMenu())
		cwd, _ := os.Getwd()
		openArg(os.Args[1:], cwd)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// openArg opens the repository named by a command line's arguments, its
// working folder's when none is, or asks for one when that isn't a
// repository either, as when the app starts from Finder or the Start menu.
func openArg(args []string, cwd string) {
	dir := cwd
	if len(args) > 0 {
		dir = args[0]
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(cwd, dir)
		}
	}
	if repo, err := git.Open(dir); err == nil {
		open(repo)
		return
	}
	go pickAndOpen()
}

func openDir(dir string) {
	repo, err := git.Open(dir)
	if err != nil {
		go mygo.Dialog.Message(mygo.MessageOptions{Title: "打不开这个文件夹", Message: err.Error()})
		return
	}
	open(repo)
}

// open shows a repository's window, the one open already if there is one.
func open(repo *git.Repo) {
	if w := windows[repo.Dir]; w != nil {
		w.Show()
		w.Focus()
		return
	}
	_ = mygo.App.AddRecentDocument(repo.Dir)
	a := newApp(repo)
	a.win = mygo.NewWindow(mygo.WindowOptions{
		Title:     repo.Name() + " — MyGit",
		Width:     1200,
		Height:    800,
		MinWidth:  720,
		MinHeight: 480,
		StateKey:  "main",
		Content:   ui.View(a.view),
	})
	windows[repo.Dir] = a.win
	a.win.OnClosed(func() { delete(windows, repo.Dir) })
	a.start()
}

// pickAndOpen asks for a repository's folder and opens it, and quits
// when none is chosen and no window is open. The dialogs wait for their
// answer: call it on a goroutine.
func pickAndOpen() {
	for {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: "打开 git 仓库", Directory: true})
		if err != nil || len(paths) == 0 {
			mygo.RunOnMain(func() {
				if len(windows) == 0 {
					mygo.App.Quit()
				}
			})
			return
		}
		repo, err := git.Open(paths[0])
		if err == nil {
			mygo.RunOnMain(func() { open(repo) })
			return
		}
		_, _ = mygo.Dialog.Message(mygo.MessageOptions{Title: "打不开这个文件夹", Message: err.Error()})
	}
}

func appMenu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "文件", Submenu: []*mygo.MenuItem{
			{Label: "打开仓库…", Accelerator: "CmdOrCtrl+O", Click: func(*mygo.MenuItem, *mygo.Window) { go pickAndOpen() }},
			{Role: mygo.RoleOpenRecent, Label: "最近打开"},
			mygo.Separator(),
			{Label: "安装命令行工具…", Click: func(*mygo.MenuItem, *mygo.Window) { go installLauncher() }},
			mygo.Separator(),
			{Role: mygo.RoleClose},
		}},
		{Role: mygo.RoleEditMenu},
		{Role: mygo.RoleWindowMenu},
	})
}
