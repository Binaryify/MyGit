package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"mygit/internal/llm"
	loc "mygit/locales"
)

// aiSettings are the model writing commit messages (#25): one provider of
// the user's, speaking OpenAI's API or Anthropic's. Its key is kept in
// the system's credential store, not here.
type aiSettings struct {
	API      llm.API `json:"api,omitempty"`     // "" for OpenAI's
	BaseURL  string  `json:"baseURL,omitempty"` // "" for the API's own provider
	Model    string  `json:"model,omitempty"`   // "" for none: no Generate button
	KeySaved bool    `json:"keySaved,omitempty"`
	// What the user asks of the messages besides, as a format or a
	// language, before the repository's way: "" for nothing more.
	Instructions string `json:"instructions,omitempty"`
	// Direct reaches the provider straight, past the proxy set, as one
	// at home may be reached, DeepSeek's from China.
	Direct bool `json:"direct,omitempty"`
}

// directHTTP is the client of the requests going past the proxy: the
// default transport's settings, without one.
var directHTTP = func() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	return &http.Client{Transport: t}
}()

// client is the HTTP client of the model's requests: through the proxy
// the app goes by, as its downloads, unless Direct, nil for that one.
func (s aiSettings) client() *http.Client {
	if s.Direct {
		return directHTTP
	}
	return nil
}

// on tells whether a model is set, which shows the Generate button.
func (s aiSettings) on() bool { return strings.TrimSpace(s.Model) != "" }

// aiKeyName is the key's in the credential store.
const aiKeyName = "ai-api-key"

// aiKey and saveAIKey read and keep the provider's key; tests replace
// them, as the credential store may ask the user. Saving "" removes it.
var (
	aiKey = func() (string, error) {
		b, err := mygo.Secrets.Get(aiKeyName)
		if errors.Is(err, mygo.ErrSecretNotFound) {
			return "", nil
		}
		return string(b), err
	}
	saveAIKey = func(key string) error {
		if key == "" {
			err := mygo.Secrets.Delete(aiKeyName)
			if errors.Is(err, mygo.ErrSecretNotFound) {
				return nil
			}
			return err
		}
		return mygo.Secrets.Set(aiKeyName, []byte(key))
	}
)

// canGenerateMessage tells whether the model can write the message: one
// is set, there are staged changes for it to read, and nothing runs.
func (a *app) canGenerateMessage() bool {
	return prefs.AI.on() && len(a.staged) > 0 && a.busy == ""
}

// generateMessage has the model write the commit message from the staged
// changes, in the way of the repository's recent ones. What it writes
// fills the box, to read before committing.
func (a *app) generateMessage() {
	if !a.canGenerateMessage() {
		return
	}
	s := prefs.AI
	var msg string
	a.run(loc.OpGenerateMessage(), func(ctx context.Context) error {
		key, err := aiKey()
		if err != nil {
			return err
		}
		stat, err := a.repo.Git(ctx, "diff", "--staged", "--no-ext-diff", "--stat")
		if err != nil {
			return err
		}
		diff, err := a.repo.Git(ctx, "diff", "--staged", "--no-ext-diff")
		if err != nil {
			return err
		}
		prompt := messagePrompt(a.repo.RecentMessages(ctx, 10), stat, diff)
		cfg := llm.Config{API: s.API, BaseURL: s.BaseURL, Model: strings.TrimSpace(s.Model), Key: key, HTTP: s.client()}
		if msg, err = llm.Complete(ctx, cfg, messageSystem(s.Instructions), prompt); err == nil && msg == "" {
			err = errors.New(loc.ErrNoMessage())
		}
		return err
	}, func() {
		a.message, a.recall = msg, 0
	})
}

// generateButton is the Generate button, with the sparkles AI tools mark
// their actions with, telling whether it was clicked.
func generateButton(c *ui.Context, disabled bool) bool {
	b := ui.Button(c, "").Label(loc.GenerateMessage()).Tooltip(loc.GenerateMessageTip()).Disabled(disabled)
	b.Children(func() {
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons["sparkles"]).Size(14, 14)
			ui.Text(c, loc.GenerateMessage())
		})
	})
	return b.Clicked()
}

// maxPromptDiff is how much of the diff the model reads, in bytes: what
// most models take with room to spare, and costs little.
const maxPromptDiff = 60_000

// messageSystem tells the model what to write: as the repository does,
// unless the user's instructions say otherwise. That the reply is the
// message alone holds whatever they say, as it fills the box as it is.
func messageSystem(instructions string) string {
	s := "You write git commit messages. Reply with the message alone: no reasoning, no preamble, no quotes, no code fence. " +
		"A summary line of at most 72 characters, then, only when the change needs explaining, a blank line and a short body. " +
		"Leave out trailers, such as Signed-off-by: or Co-authored-by:, which the user adds. " +
		"Write in the language and the style of the repository's recent commit messages; when there are none, in the language of the locale " +
		i18n.Locale() + "."
	if instructions = strings.TrimSpace(instructions); instructions != "" {
		s += "\n\nThe user's instructions, which come before the repository's style; whatever they say, reply with the message alone:\n" + instructions
	}
	return s
}

// messagePrompt gives the model the recent messages, to follow, and the
// staged changes, the diff cut short at a line when it is long.
func messagePrompt(recent []string, stat, diff string) string {
	var b strings.Builder
	if len(recent) > 0 {
		b.WriteString("Recent commit messages:\n")
		for _, m := range recent {
			fmt.Fprintf(&b, "---\n%s\n", strings.TrimSpace(m))
		}
		b.WriteString("---\n\n")
	}
	fmt.Fprintf(&b, "Staged changes:\n%s\n", strings.TrimRight(stat, "\n"))
	if len(diff) > maxPromptDiff {
		cut := strings.LastIndexByte(diff[:maxPromptDiff], '\n') + 1
		diff = diff[:cut] + "[the rest of the diff is left out]\n"
	}
	fmt.Fprintf(&b, "\n%s", diff)
	return b.String()
}

// aiEntry is the AI's line in the settings: whether it is set up, and a
// button opening its own dialog, so that the settings of everyone stay
// as they were (#25).
func (d *settingsDialog) aiEntry(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, loc.AiSection()).Bold().TextColor(t.TextMuted)
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		status := loc.AiStatusOff()
		if prefs.AI.on() {
			status = loc.AiStatusOn(strings.TrimSpace(prefs.AI.Model))
		}
		ui.Text(c, status).FontSize(12).TextColor(t.TextMuted).SingleLine().Grow(1).Basis(0)
		if ui.Button(c, loc.AiSetUp()).Clicked() {
			d.ai.open = true
		}
	})
}

// aiDialog sets the model writing commit messages, over the settings: the
// API its provider speaks, where, which model, the key, kept as it
// closes, and what the user asks of the messages besides.
type aiDialog struct {
	open   bool
	key    string // typed, kept as the dialog closes
	keyErr string // why it couldn't be kept or removed
}

func (d *aiDialog) view(c *ui.Context) {
	if !d.open {
		return
	}
	t := c.Theme()
	s := &prefs.AI
	if ui.Modal(c, &d.open, func() {
		_, h := c.Size()
		ui.Column(c).Gap(12).Width(dialogWidth(c, 440)).Children(func() {
			ui.Text(c, loc.AiSection()).Bold().FontSize(15)
			ui.Scroll(c).MaxHeight(h-t.Space(40)).Gap(8).Padding(0, scrollbarRoom, 0, 0).Children(func() {
				caption := func(text string) { ui.Text(c, text).FontSize(12).TextColor(t.TextMuted) }
				apis := []llm.API{llm.OpenAI, llm.Anthropic}
				labels := []string{loc.ApiOpenAI(), "Anthropic"}
				choice := labels[max(slices.Index(apis, s.API), 0)]
				caption(loc.ApiKind())
				if ui.Select(c.Key("ai-api"), &choice, labels).Label(loc.ApiKind()).Changed() {
					s.API = apis[slices.Index(labels, choice)]
				}
				caption(loc.BaseURL())
				ui.TextInput(c.Key("ai-base"), &s.BaseURL).Placeholder(loc.BaseURLPlaceholder(llm.DefaultBaseURL(s.API))).Label(loc.BaseURL())
				ui.Checkbox(c.Key("ai-direct"), &s.Direct, loc.AiDirect())
				ui.Text(c, loc.AiDirectHint()).FontSize(11).TextColor(t.TextMuted).Wrap()
				caption(loc.Model())
				ui.TextInput(c.Key("ai-model"), &s.Model).Placeholder(loc.ModelPlaceholder()).Label(loc.Model())
				caption(loc.ApiKey())
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					placeholder := loc.ApiKeyPlaceholder()
					if s.KeySaved {
						placeholder = loc.ApiKeySaved()
					}
					ui.TextInput(c.Key("ai-key"), &d.key).Password().Placeholder(placeholder).Label(loc.ApiKey()).Grow(1)
					if s.KeySaved && ui.Button(c, loc.DeleteAPIKey()).Clicked() {
						if err := saveAIKey(""); err != nil {
							d.keyErr = err.Error()
						} else {
							s.KeySaved, d.keyErr = false, ""
						}
					}
				})
				if d.keyErr != "" {
					ui.Text(c, d.keyErr).FontSize(11).TextColor(t.Danger).Wrap()
				}
				// Said apart, where the key is typed: it stays on this computer.
				ui.Text(c, loc.AiKeyNotice()).FontSize(11).Wrap()
				caption(loc.AiInstructions())
				ui.TextArea(c.Key("ai-instructions"), &s.Instructions).Lines(2, 6).Placeholder(loc.AiInstructionsPlaceholder()).Label(loc.AiInstructions())
				ui.Text(c, loc.AiHint()).FontSize(11).TextColor(t.TextMuted).Wrap()
			})
			ui.Row(c).Justify(ui.End).Children(func() {
				if ui.PrimaryButton(c, loc.Done()).Clicked() {
					d.open = false
				}
			})
		})
	}).Dismissed() || !d.open {
		if !d.saveKey() {
			d.open = true
			return
		}
		prefs.save()
	}
}

// saveKey keeps the key typed, as the dialog closes, telling whether it
// could: when it couldn't, the dialog stays to say why.
func (d *aiDialog) saveKey() bool {
	key := strings.TrimSpace(d.key)
	if key == "" {
		return true
	}
	if err := saveAIKey(key); err != nil {
		d.keyErr = loc.ErrSaveAPIKey(err.Error())
		return false
	}
	prefs.AI.KeySaved, d.key, d.keyErr = true, "", ""
	return true
}
