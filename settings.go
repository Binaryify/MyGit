package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"mygit/internal/sysproxy"
	loc "mygit/locales"
)

// settings are the choices kept between launches, shared by the windows,
// in a JSON file.
type settings struct {
	Editor           string           `json:"editor"`          // the command opening files, "" for the system's app
	Language         string           `json:"language"`        // the language chosen, "" for the system's
	Theme            mygo.ThemeSource `json:"theme,omitempty"` // light or dark, "" for the system's
	Unified          bool             `json:"unified"`
	IgnoreWhitespace bool             `json:"ignoreWhitespace"`
	SidebarW         float32          `json:"sidebarW"`
	LogH             float32          `json:"logH"`
	FilesW           float32          `json:"filesW"`
	StagedPct        float32          `json:"stagedPct"`         // of the changes' room, the staged changes', in percent
	Proxy            string           `json:"proxy,omitempty"`   // the downloads', "" for the system's
	Remotes          []string         `json:"remotes,omitempty"` // repositories on other computers opened lately, host:dir
	path             string
}

var prefs = &settings{SidebarW: 300, LogH: 360, FilesW: 260, StagedPct: 30}

func loadSettings() {
	dir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	prefs.path = filepath.Join(dir, "MyGit", "settings.json")
	if data, err := os.ReadFile(prefs.path); err == nil {
		_ = json.Unmarshal(data, prefs)
	}
	_ = sysproxy.Set(prefs.Proxy)
	applyTheme(prefs.Theme) // the app starts in it
}

// applyTheme makes the app light, dark or the system's; tests replace it,
// as it waits for the app's main thread.
var applyTheme = mygo.Theme.SetSource

func (s *settings) save() {
	if s.path == "" {
		return // tests
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(s.path), 0o755)
	_ = os.WriteFile(s.path, data, 0o644)
}

// diffLayout is how diffs lay their sides out, as set.
func diffLayout() editor.DiffLayout {
	if prefs.Unified {
		return editor.DiffUnified
	}
	return editor.DiffSplit
}

// editorChoices are the editors to choose, the first the system's default
// and the last any other, in the language shown.
func editorChoices() []string {
	return []string{loc.EditorSystem(), "code", "cursor", "zed", "subl", "idea", loc.EditorOther()}
}

// settingsDialog is the settings, open or not, in a repository's window
// or the welcome window.
type settingsDialog struct {
	open     bool
	proxyErr string // why the proxy written isn't one
}

func (a *app) settingsView(c *ui.Context) { a.settings.view(c, true, a.reloadDiff) }

// view shows the settings while they are open, the shortcuts too in a
// repository's window, and saves them as they close, then calls closed.
func (d *settingsDialog) view(c *ui.Context, withShortcuts bool, closed func()) {
	if !d.open {
		return
	}
	t := c.Theme()
	if ui.Modal(c, &d.open, func() {
		// The title and Done stay in view, the settings between scrolling
		// in a window too short for them all.
		_, h := c.Size()
		ui.Column(c).Gap(12).Width(dialogWidth(c, 440)).Children(func() {
			ui.Text(c, loc.Settings()).Bold().FontSize(15)
			ui.Scroll(c).MaxHeight(h - t.Space(40)).Gap(12).Children(func() {
				ui.Text(c, loc.ExternalEditor()).Bold().TextColor(t.TextMuted)
				choices := editorChoices()
				choice := prefs.Editor
				switch choice {
				case "":
					choice = choices[0]
				case "code", "cursor", "zed", "subl", "idea":
				default:
					choice = choices[len(choices)-1]
				}
				if ui.Select(c, &choice, choices).Label(loc.ExternalEditor()).Changed() {
					switch choice {
					case choices[0]:
						prefs.Editor = ""
					case choices[len(choices)-1]:
						prefs.Editor = " " // typed below
					default:
						prefs.Editor = choice
					}
				}
				if choice == choices[len(choices)-1] {
					ui.TextInput(c, &prefs.Editor).Placeholder(loc.EditorCommandPlaceholder()).Label(loc.EditorCommand())
				}
				ui.Text(c, loc.EditorLineHint()).FontSize(11).TextColor(t.TextMuted)
				ui.Text(c, loc.Language()).Bold().TextColor(t.TextMuted)
				languageSelect(c)
				ui.Text(c, loc.Appearance()).Bold().TextColor(t.TextMuted)
				themeSelect(c)
				ui.Text(c, loc.DiffSection()).Bold().TextColor(t.TextMuted)
				ui.Checkbox(c, &prefs.Unified, loc.DiffUnified())
				ui.Checkbox(c, &prefs.IgnoreWhitespace, loc.IgnoreWhitespace())
				ui.Text(c, loc.NetworkSection()).Bold().TextColor(t.TextMuted)
				if ui.TextInput(c, &prefs.Proxy).Placeholder(loc.ProxyPlaceholder()).Label(loc.Proxy()).Changed() {
					d.proxyErr = ""
					if err := sysproxy.Set(prefs.Proxy); err != nil {
						d.proxyErr = loc.ProxyInvalid()
					}
				}
				if d.proxyErr != "" {
					ui.Text(c, d.proxyErr).FontSize(11).TextColor(t.Danger)
				} else {
					ui.Text(c, loc.ProxyHint()).FontSize(11).TextColor(t.TextMuted).Wrap()
				}
				if withShortcuts {
					ui.Text(c, loc.Shortcuts()).Bold().TextColor(t.TextMuted)
					ui.Column(c).Gap(6).Children(func() {
						for _, k := range shortcuts() {
							ui.Row(c.Key(k.keys)).Gap(12).Children(func() {
								ui.Text(c, k.keys).Font("monospace").FontSize(12).Width(96)
								ui.Text(c, k.does).FontSize(12).TextColor(t.TextMuted).Grow(1).Basis(0)
							})
						}
					})
				}
			})
			ui.Row(c).Justify(ui.End).Children(func() {
				if ui.PrimaryButton(c, loc.Done()).Clicked() {
					d.open = false
				}
			})
		})
	}).Dismissed() || !d.open {
		prefs.save()
		if closed != nil {
			closed()
		}
	}
}

// keyBinding is a shortcut the window answers to, and what it does.
type keyBinding struct{ keys, does string }

// shortcuts are the keys the window answers to, as the settings list them,
// in the language shown.
func shortcuts() []keyBinding {
	return []keyBinding{
		{shortcut("K"), loc.ShortcutCommands()},
		{shortcut("P"), strings.TrimSuffix(loc.GoToFile(), "…")},
		{shortcut("F"), loc.SearchHistory()},
		{shortcut("↩"), loc.Commit()},
		{shortcut("S"), loc.ShortcutSave()},
		{"F7", loc.NextChange()},
		{shiftKey("F7"), loc.PreviousChange()},
		{shortcut("R"), loc.Refresh()},
		{shortcut("O"), strings.TrimSuffix(loc.MenuOpenRepo(), "…")},
		{shiftShortcut("O"), strings.TrimSuffix(loc.SshMenu(), "…")},
		{shortcut(","), strings.TrimSuffix(loc.SettingsMenu(), "…")},
		{loc.ShortcutSpace(), loc.ShortcutStage()},
		{loc.ShortcutDelete(), loc.ShortcutDiscard()},
		{shortcut(loc.ShortcutClick()), loc.ShortcutCompare()},
	}
}

// themeSelect picks the app's appearance: the system's, light or dark,
// for every window at once.
func themeSelect(c *ui.Context) {
	sources := []mygo.ThemeSource{mygo.ThemeSystem, mygo.ThemeLight, mygo.ThemeDark}
	labels := []string{loc.ThemeSystem(), loc.ThemeLight(), loc.ThemeDark()}
	choice := labels[max(slices.Index(sources, prefs.Theme), 0)]
	if ui.Select(c.Key("theme"), &choice, labels).Label(loc.Appearance()).Changed() {
		prefs.Theme = sources[slices.Index(labels, choice)]
		if prefs.Theme == mygo.ThemeSystem {
			prefs.Theme = ""
		}
		prefs.save()
		applyTheme(prefs.Theme)
	}
}

// languageSelect picks the language the app shows: the system's, or one
// of those it is written in, each named in itself.
func languageSelect(c *ui.Context) {
	tags := mygo.App.Languages()
	labels := []string{loc.LanguageSystem()}
	for _, tag := range tags {
		labels = append(labels, i18n.Autonym(tag))
	}
	choice := labels[0]
	if i := slices.Index(tags, prefs.Language); i >= 0 {
		choice = labels[i+1]
	}
	if ui.Select(c.Key("language"), &choice, labels).Label(loc.Language()).Changed() {
		tag := ""
		if i := slices.Index(labels, choice); i > 0 {
			tag = tags[i-1]
		}
		prefs.Language = tag
		prefs.save()
		mygo.App.SetLocale(tag)
	}
}
