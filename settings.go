package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// settings are the choices kept between launches, shared by the windows,
// in a JSON file.
type settings struct {
	Editor           string  `json:"editor"` // the command opening files, "" for the system's app
	Unified          bool    `json:"unified"`
	IgnoreWhitespace bool    `json:"ignoreWhitespace"`
	SidebarW         float32 `json:"sidebarW"`
	LogH             float32 `json:"logH"`
	FilesW           float32 `json:"filesW"`
	path             string
}

var prefs = &settings{SidebarW: 300, LogH: 360, FilesW: 260}

func loadSettings() {
	dir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	prefs.path = filepath.Join(dir, "MyGit", "settings.json")
	if data, err := os.ReadFile(prefs.path); err == nil {
		_ = json.Unmarshal(data, prefs)
	}
}

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

var editorChoices = []string{"系统默认", "code", "cursor", "zed", "subl", "idea", "其他…"}

func (a *app) settingsView(c *ui.Context) {
	if !a.settingsOpen {
		return
	}
	t := c.Theme()
	if ui.Modal(c, &a.settingsOpen, func() {
		ui.Column(c).Gap(12).Width(440).Children(func() {
			ui.Text(c, "设置").Bold().FontSize(15)
			ui.Text(c, "外部编辑器").Bold().TextColor(t.TextMuted)
			choice := prefs.Editor
			switch choice {
			case "":
				choice = editorChoices[0]
			case "code", "cursor", "zed", "subl", "idea":
			default:
				choice = editorChoices[len(editorChoices)-1]
			}
			if ui.Select(c, &choice, editorChoices).Label("外部编辑器").Changed() {
				switch choice {
				case editorChoices[0]:
					prefs.Editor = ""
				case editorChoices[len(editorChoices)-1]:
					prefs.Editor = " " // typed below
				default:
					prefs.Editor = choice
				}
			}
			if choice == editorChoices[len(editorChoices)-1] {
				ui.TextInput(c, &prefs.Editor).Placeholder("命令或程序路径，如 /usr/local/bin/nvim-qt").Label("编辑器命令")
			}
			ui.Text(c, "命令行编辑器（code、zed 等）会跳到对应的行。").FontSize(11).TextColor(t.TextMuted)
			ui.Text(c, "差异").Bold().TextColor(t.TextMuted)
			ui.Checkbox(c, &prefs.Unified, "上下对比（不勾选为左右并排）")
			ui.Checkbox(c, &prefs.IgnoreWhitespace, "忽略空白的改动")
			ui.Row(c).Justify(ui.End).Children(func() {
				if ui.PrimaryButton(c, "完成").Clicked() {
					a.settingsOpen = false
				}
			})
		})
	}).Dismissed() || !a.settingsOpen {
		prefs.save()
		a.reloadDiff()
	}
}
