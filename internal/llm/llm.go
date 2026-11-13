// Package llm asks a language model for a reply, through one of the two
// APIs most providers speak: OpenAI's Chat Completions, and Anthropic's
// Messages. It knows no provider: the user gives the address, the model
// and the key.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// API is the protocol a provider speaks.
type API string

const (
	OpenAI    API = "openai"    // POST <base>/chat/completions
	Anthropic API = "anthropic" // POST <base>/v1/messages
)

// DefaultBaseURL is the address of the API's own provider, for a config
// that gives none.
func DefaultBaseURL(api API) string {
	if api == Anthropic {
		return "https://api.anthropic.com"
	}
	return "https://api.openai.com/v1"
}

// Config is where a model is and how to reach it.
type Config struct {
	API     API
	BaseURL string // "" for DefaultBaseURL's
	Model   string
	Key     string       // "" for none, as a local server takes
	HTTP    *http.Client // nil for http.DefaultClient
}

// Complete sends a system prompt and the user's, and gives the model's
// reply, its reasoning and code fences left out.
func Complete(ctx context.Context, c Config, system, user string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if base == "" {
		base = DefaultBaseURL(c.API)
	}
	var (
		url   string
		body  any
		parse func([]byte) (string, error)
	)
	header := http.Header{"Content-Type": {"application/json"}}
	if c.API == Anthropic {
		url = base + "/v1/messages"
		if c.Key != "" {
			// Anthropic's own takes x-api-key, before a bearer; many
			// providers speaking its API, as Ollama's, the bearer alone.
			header.Set("x-api-key", c.Key)
			header.Set("Authorization", "Bearer "+c.Key)
		}
		header.Set("anthropic-version", "2023-06-01")
		body = map[string]any{
			"model":      c.Model,
			"max_tokens": 4096,
			"system":     system,
			"messages":   []map[string]string{{"role": "user", "content": user}},
		}
		parse = anthropicText
	} else {
		url = base + "/chat/completions"
		if c.Key != "" {
			header.Set("Authorization", "Bearer "+c.Key)
		}
		body = map[string]any{
			"model": c.Model,
			"messages": []map[string]string{
				{"role": "system", "content": system},
				{"role": "user", "content": user},
			},
		}
		parse = openAIText
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header = header
	h := c.HTTP
	if h == nil {
		h = http.DefaultClient
	}
	resp, err := h.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 300 {
		return "", apiError(resp.Status, out)
	}
	text, err := parse(out)
	if err != nil {
		return "", err
	}
	return clean(text), nil
}

func openAIText(data []byte) (string, error) {
	var r struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return "", fmt.Errorf("unexpected reply: %w", err)
	}
	if len(r.Choices) == 0 {
		return "", errors.New("the reply has no choices")
	}
	return r.Choices[0].Message.Content, nil
}

// anthropicText joins the reply's text blocks, leaving its thinking out.
func anthropicText(data []byte) (string, error) {
	var r struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return "", fmt.Errorf("unexpected reply: %w", err)
	}
	var b strings.Builder
	for _, block := range r.Content {
		if block.Type == "text" {
			b.WriteString(block.Text)
		}
	}
	return b.String(), nil
}

// apiError says what went wrong: the message of the error both APIs
// reply with, else the status and the start of the body.
func apiError(status string, body []byte) error {
	var r struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &r) == nil && r.Error.Message != "" {
		return fmt.Errorf("%s: %s", status, r.Error.Message)
	}
	text := strings.TrimSpace(string(body))
	if len(text) > 300 {
		text = text[:300] + "…"
	}
	if text == "" {
		return errors.New(status)
	}
	return fmt.Errorf("%s: %s", status, text)
}

var (
	thinkRE = regexp.MustCompile(`(?s)<think>.*?</think>`)
	fenceRE = regexp.MustCompile("(?s)^```[\\w-]*\\n(.*?)\\n?```$")
)

// clean leaves out what reasoning models put before their reply, and
// the code fence some put around it.
func clean(s string) string {
	s = strings.TrimSpace(thinkRE.ReplaceAllString(s, ""))
	if m := fenceRE.FindStringSubmatch(s); m != nil {
		s = strings.TrimSpace(m[1])
	}
	return s
}
