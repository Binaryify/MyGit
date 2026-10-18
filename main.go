// mygit is a light git client: the history with its graph, the working
// tree's changes to stage and commit, and the diffs of both.
//
//	mygit [dir]   open the repository dir is in, the current folder's by default
package main

import (
	"log"
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
)

func main() {
	mygo.App.SetName("mygit")
	dir := "."
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	mygo.App.WhenReady(func() {
		if repo, err := git.Open(dir); err == nil {
			open(repo)
			return
		}
		go func() { // the dialogs wait for their answer: not on the main thread
			repo := pickRepo()
			mygo.RunOnMain(func() {
				if repo == nil {
					mygo.App.Quit()
					return
				}
				open(repo)
			})
		}()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func open(repo *git.Repo) {
	a := newApp(repo)
	a.win = mygo.NewWindow(mygo.WindowOptions{
		Title:     repo.Name() + " — mygit",
		Width:     1200,
		Height:    800,
		MinWidth:  720,
		MinHeight: 480,
		StateKey:  "main",
		Content:   ui.View(a.view),
	})
	a.start()
}

// pickRepo asks for a repository's folder, as when the app is opened from
// Finder or the Start menu rather than in one, and nil if none is chosen.
func pickRepo() *git.Repo {
	for {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: "打开 git 仓库", Directory: true})
		if err != nil || len(paths) == 0 {
			return nil
		}
		repo, err := git.Open(paths[0])
		if err == nil {
			return repo
		}
		_, _ = mygo.Dialog.Message(mygo.MessageOptions{Title: "打不开这个文件夹", Message: err.Error()})
	}
}
