package main

import (
	"context"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	loc "mygit/locales"
)

// updates is what the app knows of a newer version, as MonitorBar has it:
// asked of the releases soon after launch and every few hours, shown in
// every window, and installed when the user says so.
var updates struct {
	available  *mygo.Update // nil when up to date
	installing bool
	progress   int // percent
}

// watchUpdates checks for a newer version soon after launch, then every
// few hours, in builds that can update themselves.
func watchUpdates() {
	if !mygo.Updater.Enabled() {
		return
	}
	go func() {
		time.Sleep(10 * time.Second)
		for {
			checkUpdate(nil)
			time.Sleep(6 * time.Hour)
		}
	}()
}

// checkUpdate asks the releases for a newer version; report, when given,
// hears what a check the user asked for found.
func checkUpdate(report func(string)) {
	if !mygo.Updater.Enabled() || updates.installing {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		up, err := mygo.Updater.Check(ctx)
		mygo.RunOnMain(func() {
			if err == nil {
				updates.available = up
			}
			if report != nil {
				switch {
				case err != nil:
					report(loc.UpdateCheckFailed())
				case up == nil:
					report(loc.UpdateLatest(mygo.App.Version()))
				}
			}
			redrawAll()
		})
	}()
}

// installUpdate downloads the update, checks its signature, replaces the
// app and starts the new version.
func installUpdate(fail func(string)) {
	up := updates.available
	if up == nil || updates.installing {
		return
	}
	updates.installing, updates.progress = true, 0
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		shown := -1
		err := up.Install(ctx, func(done, total int64) {
			if total <= 0 {
				return
			}
			if p := int(done * 100 / total); p != shown {
				shown = p
				mygo.RunOnMain(func() { updates.progress = p; redrawAll() })
			}
		})
		mygo.RunOnMain(func() {
			if err != nil {
				updates.installing = false
				fail(loc.UpdateFailed())
				redrawAll()
				return
			}
			prefs.save()
			mygo.App.Relaunch()
		})
	}()
}

// redrawAll draws the windows again, as what they show of the update
// changed.
func redrawAll() {
	for _, a := range apps {
		a.win.Invalidate()
	}
	if welcomeWin != nil {
		welcomeWin.win.Invalidate()
	}
}

// updateBar offers the newer version, under a repository's title bar.
func (a *app) updateBar(c *ui.Context) {
	up := updates.available
	if up == nil {
		return
	}
	t := c.Theme()
	ui.Row(c).Padding(6, 12).Gap(8).AlignItems(ui.Center).Background(t.Accent.Alpha(0.12)).Children(func() {
		ui.Text(c, loc.UpdateAvailable(up.Version)).Grow(1)
		if updates.installing {
			ui.Spinner(c).Size(12, 12)
			ui.Text(c, loc.UpdateInstalling(updates.progress)).TextColor(t.TextMuted)
			return
		}
		if ui.PrimaryButton(c, loc.UpdateInstall()).Clicked() {
			installUpdate(func(msg string) { a.errMsg = msg })
		}
	})
}

// about tells the version. The dialog waits: call it on a goroutine.
func about() {
	_, _ = mygo.Dialog.Message(mygo.MessageOptions{
		Title:   loc.About(),
		Message: "MyGit " + mygo.App.Version(),
		Detail:  loc.AboutDetail(),
	})
}
