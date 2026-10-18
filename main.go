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

	"mygit/internal/askpass"
	"mygit/internal/git"
)

// apps are the open repositories' windows, by working tree.
var apps = map[string]*app{}

func main() {
	// Run by git for a prompt, hand it to the running app.
	if code, ok := askpass.Main(os.Args); ok {
		os.Exit(code)
	}
	mygo.App.SetName("MyGit")
	loadSettings()
	if !mygo.App.RequestSingleInstanceLock() {
		return // the running MyGit opens it
	}
	mygo.App.OnSecondInstance(func(args []string, cwd string) { openArg(args, cwd) })
	mygo.App.OnOpenFile(func(path string) { openDir(path) }) // Open Recent, the Dock menu
	mygo.App.OnActivate(func(visible bool) {
		if !visible {
			showWelcome() // the Dock icon clicked with no window open
		}
	})
	mygo.App.WhenReady(func() {
		mygo.App.SetMenu(appMenu())
		if s, err := askpass.Listen(askUser); err == nil {
			git.Env = func(dir string) []string { return s.Env(dir) }
			mygo.App.OnQuit(func() { s.Close() })
		}
		cwd, _ := os.Getwd()
		openArg(os.Args[1:], cwd)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// openArg opens the repository named by a command line's arguments, its
// working folder's when none is, or the welcome window when that isn't a
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
	showWelcome()
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
	if a := apps[repo.Dir]; a != nil {
		a.win.Show()
		a.win.Focus()
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
	apps[repo.Dir] = a
	a.win.OnClosed(func() { delete(apps, repo.Dir) })
	a.start()
	closeWelcome()
}

// pickAndOpen asks for a repository's folder and opens it. The dialogs
// wait for their answer: call it on a goroutine.
func pickAndOpen() {
	for {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: "打开 git 仓库", Directory: true})
		if err != nil || len(paths) == 0 {
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

// askUser asks a prompt of git's in the window of the repository it runs
// in, or the welcome window, cloning, and waits for the answer.
func askUser(dir, prompt string) (string, bool) {
	done := make(chan credAnswer, 1)
	mygo.RunOnMain(func() {
		switch {
		case apps[dir] != nil:
			apps[dir].askCredential(prompt, done)
		case welcomeWin != nil:
			welcomeWin.cred.ask(welcomeWin.win, prompt, done)
		default:
			done <- credAnswer{}
		}
	})
	r := <-done
	return r.value, r.ok
}
