// MyGit is a light git client: the history with its graph, the working
// tree's changes to stage and commit, and the diffs of both.
//
//	mygit [dir]   open the repository dir is in, the current folder's by default
//
// A running MyGit opens the repositories later launches name, each in a
// window of its own.
package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"slices"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mygit/internal/askpass"
	"mygit/internal/git"
	"mygit/internal/sysproxy"
	loc "mygit/locales"
)

// apps are the open repositories' windows, by working tree.
var apps = map[string]*app{}

// ready tells whether the app can make windows, and pending holds the
// folders the system asked to open before it could.
var (
	ready   bool
	pending []string
)

func main() {
	// Run by git for a prompt, hand it to the running app.
	if code, ok := askpass.Main(os.Args); ok {
		os.Exit(code)
	}
	mygo.App.SetName("MyGit")
	sysproxy.Install() // downloads, the helpers' and the updates', through the system's proxy
	loadSettings()
	if !mygo.App.RequestSingleInstanceLock() {
		return // the running MyGit opens it
	}
	mygo.App.OnSecondInstance(func(args []string, cwd string) { openArg(args, cwd) })
	// Folders dropped on the icon, Open Recent, the Dock menu: those that
	// come as the app starts wait for it to be ready.
	mygo.App.OnOpenFile(func(path string) {
		if ready {
			openDir(path)
		} else {
			pending = append(pending, path)
		}
	})
	mygo.App.OnActivate(func(visible bool) {
		if !visible {
			showWelcome() // the Dock icon clicked with no window open
		}
	})
	mygo.App.WhenReady(func() {
		mygo.App.SetMenu(appMenu())
		mygo.App.OnLocaleChanged(func(string) { mygo.App.SetMenu(appMenu()) }) // its labels, in the new language
		if s, err := askpass.Listen(askUser); err == nil {
			askServer = s
			git.Env = func(dir string) []string { return s.Env(dir) }
			mygo.App.OnQuit(func() { s.Close() })
		}
		mygo.App.OnQuit(closeHosts)
		// Quitting with a file edited and not saved asks, in its window,
		// a window at a time; quitting goes on once each is saved or given up.
		mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
			for _, a := range allApps() {
				if a.unsaved() {
					e.PreventDefault()
					if a.ws != nil {
						a.ws.activate(slices.Index(a.ws.apps, a))
					}
					a.win.Focus()
					a.leaving(mygo.App.Quit)
					return
				}
			}
		})
		ready = true
		if prefs.Language != "" { // the language chosen in Settings
			mygo.App.SetLocale(prefs.Language)
		}
		watchUpdates()
		if len(pending) > 0 { // started to open them
			for _, p := range pending {
				openDir(p)
			}
			return
		}
		cwd, _ := os.Getwd()
		openArg(os.Args[1:], cwd)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// openArg opens the repository named by a command line's arguments, or
// the repositories under the folder they name; with none, the repository
// of its working folder, or the welcome window when that isn't in one, as
// when the app starts from Finder or the Start menu, in /, where it doesn't
// look for repositories below.
func openArg(args []string, cwd string) {
	if len(args) > 0 {
		if host, dir, ok := parseLocation(args[0]); ok && !exists(filepath.Join(cwd, args[0])) {
			showWelcome()
			welcomeWin.connect(host, dir)
			return
		}
	}
	dir := cwd
	if len(args) > 0 {
		dir = args[0]
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(cwd, dir)
		}
	}
	var err error
	if len(args) > 0 {
		err = openLocal(dir)
	} else if repo, rerr := git.Open(dir); rerr == nil {
		open(repo)
	} else {
		err = rerr
	}
	if err == nil {
		return
	}
	showWelcome()
	if len(args) > 0 { // a folder asked for that didn't open: say why
		welcomeWin.failed(dir, err)
	}
}

func openDir(dir string) {
	if err := openLocal(dir); err != nil {
		showWelcome()
		welcomeWin.failed(dir, err)
	}
}

// open shows a repository's window, the one open already if there is one.
func open(repo *git.Repo) {
	key := repo.Location()
	if a := apps[key]; a != nil {
		a.win.Show()
		a.win.Focus()
		return
	}
	title := repo.Name() + " — MyGit"
	if repo.Host() == "" {
		_ = mygo.App.AddRecentDocument(repo.Dir)
	} else {
		addRemoteRecent(key)
		title = repo.Name() + " — " + repo.Host()
	}
	a := newApp(repo)
	a.win = mygo.NewWindow(customTitleBar(mygo.WindowOptions{
		Title:     title,
		Width:     1200,
		Height:    800,
		MinWidth:  720,
		MinHeight: 480,
		StateKey:  "main",
		Content:   ui.View(a.view),
	}))
	dropMenuBar(a.win)
	apps[key] = a
	a.win.OnClosed(func() { delete(apps, key) })
	a.start()
	closeWelcome()
}

// pickAndOpen asks for a repository's folder and opens it. The dialogs
// wait for their answer: call it on a goroutine.
func pickAndOpen() {
	for {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: loc.OpenRepoDialog(), Directory: true})
		if err != nil || len(paths) == 0 {
			return
		}
		whole, repos, err := openFolders(context.Background(), git.Local, listLocal, paths[0])
		if err == nil {
			mygo.RunOnMain(func() { show("", whole, repos) })
			return
		}
		_, _ = mygo.Dialog.Message(mygo.MessageOptions{Title: loc.CantOpenFolder(), Message: err.Error()})
	}
}

func appMenu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: loc.MenuFile(), Submenu: []*mygo.MenuItem{
			{Label: loc.MenuOpenRepo(), Accelerator: "CmdOrCtrl+O", Click: func(*mygo.MenuItem, *mygo.Window) { go pickAndOpen() }},
			{Label: loc.SshMenu(), Accelerator: "CmdOrCtrl+Shift+O", Click: func(*mygo.MenuItem, *mygo.Window) { openSSH() }},
			{Role: mygo.RoleOpenRecent, Label: loc.MenuRecent()},
			mygo.Separator(),
			{Label: loc.MenuInstallCLI(), Click: func(*mygo.MenuItem, *mygo.Window) { go installLauncher() }},
			{Label: loc.MenuUninstallCLI(), Click: func(*mygo.MenuItem, *mygo.Window) { go uninstallLauncher() }},
			mygo.Separator(),
			{Role: mygo.RoleClose},
		}},
		{Role: mygo.RoleEditMenu, Label: loc.MenuEdit()},
		{Role: mygo.RoleWindowMenu, Label: loc.MenuWindow()},
	})
}

// exists tells whether there is a file or a folder at p.
func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// askUser asks a prompt of git's in the window of the repository it runs
// in, host:dir on another computer, or the welcome window, cloning or
// connecting, and waits for the answer.
func askUser(dir, prompt string) (string, bool) {
	done := make(chan credAnswer, 1)
	mygo.RunOnMain(func() {
		switch {
		case appAt(dir) != nil:
			a := appAt(dir)
			if a.ws != nil {
				a.ws.activate(slices.Index(a.ws.apps, a))
			}
			a.askCredential(prompt, done)
		case welcomeWin != nil:
			welcomeWin.cred.ask(welcomeWin.win, prompt, done)
		default:
			done <- credAnswer{}
		}
	})
	r := <-done
	return r.value, r.ok
}
