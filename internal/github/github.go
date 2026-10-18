// Package github reads and reviews a repository's pull requests with
// GitHub's API: the open ones, a pull request's review threads, replies,
// resolving threads, and reviews with their comments.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Client is a repository's API, with the user's token when there is one:
// public pull requests read without one, and reviewing needs it.
type Client struct {
	Owner, Repo string
	Token       string
	BaseURL     string // https://api.github.com, else a test's server
	HTTP        *http.Client
}

// remoteRE takes a GitHub remote's owner and repository from its URL, as
// git@github.com:o/r.git, https://github.com/o/r and ssh://git@github.com/o/r.
var remoteRE = regexp.MustCompile(`github\.com[:/]+([^/]+)/([^/]+?)(?:\.git)?/?$`)

// ParseRemote is the owner and repository a GitHub remote's URL names.
func ParseRemote(url string) (owner, repo string, ok bool) {
	m := remoteRE.FindStringSubmatch(strings.TrimSpace(url))
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// Token is the user's GitHub token: $GH_TOKEN, $GITHUB_TOKEN, else the
// GitHub CLI's, as gh auth login keeps it; "" when there is none.
func Token(ctx context.Context) string {
	for _, k := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if t := strings.TrimSpace(os.Getenv(k)); t != "" {
			return t
		}
	}
	gh, err := exec.LookPath("gh")
	if err != nil {
		return ""
	}
	out, err := exec.CommandContext(ctx, gh, "auth", "token", "--hostname", "github.com").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// New is a client of owner/repo's API with the user's token.
func New(ctx context.Context, owner, repo string) *Client {
	return &Client{Owner: owner, Repo: repo, Token: Token(ctx)}
}

// ErrNoToken says reviewing needs the user to sign in.
var ErrNoToken = errors.New("no GitHub token")

// User is an account, as a pull request's author.
type User struct {
	Login string `json:"login"`
}

// Branch is a side of a pull request: its branch and commit.
type Branch struct {
	Ref   string `json:"ref"`
	SHA   string `json:"sha"`
	Label string `json:"label"` // owner:branch
}

// Pull is a pull request.
type Pull struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	Draft     bool      `json:"draft"`
	Merged    bool      `json:"merged"`
	User      User      `json:"user"`
	Head      Branch    `json:"head"`
	Base      Branch    `json:"base"`
	HTMLURL   string    `json:"html_url"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Comment is a review comment on lines of a pull request's file. Line is
// nil for a comment the pull request's commits since moved past
// (outdated); Side is LEFT for the old file's lines, RIGHT for the new's.
type Comment struct {
	ID        int64     `json:"id"`
	InReplyTo int64     `json:"in_reply_to_id"`
	Path      string    `json:"path"`
	Line      *int      `json:"line"`
	StartLine *int      `json:"start_line"`
	Side      string    `json:"side"`
	Body      string    `json:"body"`
	User      User      `json:"user"`
	CreatedAt time.Time `json:"created_at"`
	HTMLURL   string    `json:"html_url"`
}

// Thread is a review comment and its replies, and whether it is resolved:
// ID, its GraphQL id to resolve it, is "" when read without a token.
type Thread struct {
	ID       string
	Resolved bool
	Comments []Comment
}

// Root is the thread's first comment, which places it.
func (t Thread) Root() Comment { return t.Comments[0] }

// DraftComment is a comment of a review being submitted, on lines of the
// new file (RIGHT) or the old (LEFT), StartLine 0 for one line.
type DraftComment struct {
	Path      string `json:"path"`
	Body      string `json:"body"`
	Line      int    `json:"line"`
	Side      string `json:"side"`
	StartLine int    `json:"start_line,omitempty"`
	StartSide string `json:"start_side,omitempty"`
}

// Review events: what a review says of the pull request.
const (
	ReviewComment        = "COMMENT"
	ReviewApprove        = "APPROVE"
	ReviewRequestChanges = "REQUEST_CHANGES"
)

// Pulls lists the open pull requests, those updated last first.
func (c *Client) Pulls(ctx context.Context) ([]Pull, error) {
	var pulls []Pull
	err := c.do(ctx, "GET", c.repoPath("/pulls?state=open&sort=updated&direction=desc&per_page=50"), nil, &pulls)
	return pulls, err
}

// Pull reads a pull request.
func (c *Client) Pull(ctx context.Context, n int) (Pull, error) {
	var p Pull
	err := c.do(ctx, "GET", c.repoPath(fmt.Sprintf("/pulls/%d", n)), nil, &p)
	return p, err
}

// Threads reads a pull request's review comments as threads, in the order
// they began, with whether each is resolved when there is a token.
func (c *Client) Threads(ctx context.Context, n int) ([]Thread, error) {
	var all []Comment
	for page := 1; ; page++ {
		var batch []Comment
		if err := c.do(ctx, "GET", c.repoPath(fmt.Sprintf("/pulls/%d/comments?per_page=100&page=%d", n, page)), nil, &batch); err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < 100 {
			break
		}
	}
	var threads []Thread
	at := map[int64]int{} // a comment's thread
	for _, cm := range all {
		if i, ok := at[cm.InReplyTo]; ok && cm.InReplyTo != 0 {
			threads[i].Comments = append(threads[i].Comments, cm)
			at[cm.ID] = i
			continue
		}
		at[cm.ID] = len(threads)
		threads = append(threads, Thread{Comments: []Comment{cm}})
	}
	if c.Token != "" {
		if state, err := c.threadState(ctx, n); err == nil {
			for i := range threads {
				if s, ok := state[threads[i].Root().ID]; ok {
					threads[i].ID, threads[i].Resolved = s.id, s.resolved
				}
			}
		}
	}
	return threads, nil
}

type threadState struct {
	id       string
	resolved bool
}

// threadState reads, with GraphQL, which threads are resolved, by their
// first comments.
func (c *Client) threadState(ctx context.Context, n int) (map[int64]threadState, error) {
	const q = `query($owner: String!, $repo: String!, $n: Int!, $after: String) {
  repository(owner: $owner, name: $repo) { pullRequest(number: $n) {
    reviewThreads(first: 100, after: $after) {
      pageInfo { hasNextPage endCursor }
      nodes { id isResolved comments(first: 1) { nodes { databaseId } } }
    } } } }`
	state := map[int64]threadState{}
	var after *string
	for {
		var out struct {
			Repository struct {
				PullRequest struct {
					ReviewThreads struct {
						PageInfo struct {
							HasNextPage bool
							EndCursor   string
						}
						Nodes []struct {
							ID         string
							IsResolved bool
							Comments   struct {
								Nodes []struct{ DatabaseID int64 }
							}
						}
					}
				}
			}
		}
		vars := map[string]any{"owner": c.Owner, "repo": c.Repo, "n": n, "after": after}
		if err := c.graphql(ctx, q, vars, &out); err != nil {
			return nil, err
		}
		rt := out.Repository.PullRequest.ReviewThreads
		for _, t := range rt.Nodes {
			if len(t.Comments.Nodes) > 0 {
				state[t.Comments.Nodes[0].DatabaseID] = threadState{t.ID, t.IsResolved}
			}
		}
		if !rt.PageInfo.HasNextPage {
			return state, nil
		}
		cursor := rt.PageInfo.EndCursor
		after = &cursor
	}
}

// Reply answers a thread, by its first comment.
func (c *Client) Reply(ctx context.Context, n int, root int64, body string) (Comment, error) {
	var cm Comment
	err := c.do(ctx, "POST", c.repoPath(fmt.Sprintf("/pulls/%d/comments/%d/replies", n, root)), map[string]any{"body": body}, &cm)
	return cm, err
}

// Resolve resolves a thread, or opens it again.
func (c *Client) Resolve(ctx context.Context, thread string, resolved bool) error {
	m := "unresolveReviewThread"
	if resolved {
		m = "resolveReviewThread"
	}
	q := fmt.Sprintf(`mutation($id: ID!) { %s(input: {threadId: $id}) { thread { isResolved } } }`, m)
	return c.graphql(ctx, q, map[string]any{"id": thread}, nil)
}

// Comment posts a comment alone, outside a review, on the pull request's
// commit (its head).
func (c *Client) Comment(ctx context.Context, n int, commit string, d DraftComment) (Comment, error) {
	body := map[string]any{"commit_id": commit, "path": d.Path, "body": d.Body, "line": d.Line, "side": d.Side}
	if d.StartLine != 0 {
		body["start_line"], body["start_side"] = d.StartLine, d.StartSide
	}
	var cm Comment
	err := c.do(ctx, "POST", c.repoPath(fmt.Sprintf("/pulls/%d/comments", n)), body, &cm)
	return cm, err
}

// Review submits a review of the pull request's commit, with its
// comments, as a comment, an approval or a request for changes.
func (c *Client) Review(ctx context.Context, n int, commit, body, event string, comments []DraftComment) error {
	req := map[string]any{"commit_id": commit, "body": body, "event": event, "comments": comments}
	if comments == nil {
		req["comments"] = []DraftComment{}
	}
	return c.do(ctx, "POST", c.repoPath(fmt.Sprintf("/pulls/%d/reviews", n)), req, nil)
}

func (c *Client) repoPath(p string) string {
	return "/repos/" + c.Owner + "/" + c.Repo + p
}

// Error is what GitHub answered a request it refused with.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("GitHub: %d", e.Status)
	}
	return fmt.Sprintf("GitHub: %s (%d)", e.Message, e.Status)
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	if method != "GET" && c.Token == "" {
		return ErrNoToken
	}
	base := c.BaseURL
	if base == "" {
		base = "https://api.github.com"
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	h := c.HTTP
	if h == nil {
		h = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := h.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
			Errors  []any  `json:"errors"`
		}
		json.Unmarshal(data, &e)
		msg := e.Message
		if len(e.Errors) > 0 {
			msg += fmt.Sprintf(": %v", e.Errors)
		}
		return &Error{resp.StatusCode, msg}
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

func (c *Client) graphql(ctx context.Context, query string, vars map[string]any, out any) error {
	if c.Token == "" {
		return ErrNoToken
	}
	var resp struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := c.do(ctx, "POST", "/graphql", map[string]any{"query": query, "variables": vars}, &resp); err != nil {
		return err
	}
	if len(resp.Errors) > 0 {
		return &Error{200, resp.Errors[0].Message}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(resp.Data, out)
}
