// Package search finds text across session transcripts.
//
// It scans each transcript's raw lines, cheaply filters them on the query,
// and parses only the lines that might match, so searching many large
// transcripts does not mean parsing all of them.
package search

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/gsperry/agent-desk/internal/hub"
	"github.com/gsperry/agent-desk/internal/locate"
	"github.com/gsperry/agent-desk/internal/transcript"
)

type Hit struct {
	SessionID   string          `json:"sessionId"`
	SessionName string          `json:"sessionName"`
	Project     string          `json:"project"`
	Archived    bool            `json:"archived,omitempty"`
	UUID        string          `json:"uuid"`
	Kind        transcript.Kind `json:"kind"`
	Tool        string          `json:"tool,omitempty"`
	Time        string          `json:"time,omitempty"`
	Snippet     string          `json:"snippet"`
}

type Result struct {
	Query     string `json:"query"`
	Hits      []Hit  `json:"hits"`
	Truncated bool   `json:"truncated"`
	Searched  int    `json:"searched"`
}

type Options struct {
	IncludeArchived bool
	Limit           int
}

type Sessions interface {
	State() hub.State
}

type Searcher struct {
	Sessions Sessions
	Locator  *locate.Locator
}

var ErrQuery = errors.New("search for at least 2 characters and at most 200")

const (
	perSession   = 20
	defaultLimit = 100
	snippetRunes = 160
	workers      = 4
)

func (s *Searcher) Search(ctx context.Context, query string, opts Options) (Result, error) {
	q := strings.TrimSpace(query)
	if n := utf8.RuneCountInString(q); n < 2 || n > 200 {
		return Result{}, ErrQuery
	}
	limit := opts.Limit
	if limit <= 0 || limit > 500 {
		limit = defaultLimit
	}
	var targets []hub.SessionView
	for _, sess := range s.Sessions.State().Sessions {
		if sess.Tool != "claude" || sess.AgentSessionID == "" || (sess.Archived && !opts.IncludeArchived) {
			continue
		}
		targets = append(targets, sess)
	}

	m := newMatcher(q)
	var mu sync.Mutex
	var hits []Hit
	searched := 0
	jobs := make(chan hub.SessionView)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for sess := range jobs {
				found, ok := s.searchSession(ctx, sess, m)
				mu.Lock()
				if ok {
					searched++
				}
				hits = append(hits, found...)
				mu.Unlock()
			}
		}()
	}
	for _, t := range targets {
		select {
		case jobs <- t:
		case <-ctx.Done():
		}
	}
	close(jobs)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].Time > hits[j].Time })
	res := Result{Query: q, Searched: searched, Hits: hits}
	if len(hits) > limit {
		res.Hits, res.Truncated = hits[:limit], true
	}
	if res.Hits == nil {
		res.Hits = []Hit{}
	}
	return res, nil
}

func (s *Searcher) searchSession(ctx context.Context, sess hub.SessionView, m matcher) ([]Hit, bool) {
	path, err := s.Locator.Find(ctx, sess.AgentSessionID, sess.Environment)
	if err != nil {
		return nil, false
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 1<<20)
	p := transcript.NewParser()
	var hits []Hit
	for n := 0; ; n++ {
		if n%2000 == 0 && ctx.Err() != nil {
			return hits, true
		}
		line, err := r.ReadBytes('\n')
		if len(line) > 0 && m.maybe(line) {
			for _, it := range p.Line(line) {
				text := searchable(it)
				if i := m.index(text); i >= 0 {
					hits = append(hits, Hit{
						SessionID:   sess.ID,
						SessionName: sess.Name,
						Project:     sess.Project.Name,
						Archived:    sess.Archived,
						UUID:        it.UUID,
						Kind:        it.Kind,
						Tool:        toolName(it),
						Time:        it.Time,
						Snippet:     snippet(text, i, len(m.lower)),
					})
					if len(hits) >= perSession {
						return hits, true
					}
				}
			}
		}
		if err == io.EOF {
			return hits, true
		}
		if err != nil {
			return hits, true
		}
	}
}

// matcher holds the lowered query and a token for the raw-line filter. JSON
// escapes quotes, backslashes, newlines and some Unicode, so the filter
// uses the longest run of characters that appear in raw JSON as written.
type matcher struct {
	lower string
	token []byte
}

func newMatcher(q string) matcher {
	lower := strings.ToLower(q)
	best := ""
	for _, run := range strings.FieldsFunc(lower, func(r rune) bool {
		return r == '"' || r == '\\' || r == '\n' || r == '\t' || r < 0x20 || r > 0x7e || r == '<' || r == '>' || r == '&'
	}) {
		if len(run) > len(best) {
			best = run
		}
	}
	return matcher{lower: lower, token: []byte(best)}
}

func (m matcher) maybe(line []byte) bool {
	if len(m.token) == 0 {
		return true
	}
	return bytes.Contains(bytes.ToLower(line), m.token)
}

func (m matcher) index(text string) int {
	if text == "" {
		return -1
	}
	return strings.Index(strings.ToLower(text), m.lower)
}

// searchable is the text of an item a reader would search for.
func searchable(it transcript.Item) string {
	switch it.Kind {
	case transcript.KindUser, transcript.KindText, transcript.KindThinking, transcript.KindPeer,
		transcript.KindCompactSummary, transcript.KindNotice, transcript.KindSystem:
		return it.Text
	case transcript.KindToolUse:
		if it.Tool == nil {
			return ""
		}
		return strings.Join(append([]string{it.Tool.Name}, stringsIn(it.Tool.Input)...), "\n")
	case transcript.KindToolResult:
		if it.Result == nil {
			return ""
		}
		return it.Result.Content
	}
	return ""
}

func toolName(it transcript.Item) string {
	if it.Tool != nil {
		return it.Tool.Name
	}
	return ""
}

// stringsIn collects the string values of a JSON document, so a search
// matches tool input as written rather than as escaped JSON.
func stringsIn(raw json.RawMessage) []string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return nil
	}
	var out []string
	var walk func(any)
	walk = func(v any) {
		switch t := v.(type) {
		case string:
			out = append(out, t)
		case []any:
			for _, c := range t {
				walk(c)
			}
		case map[string]any:
			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				walk(t[k])
			}
		}
	}
	walk(v)
	return out
}

// snippet is the text around a match, on one line, cut at rune boundaries.
func snippet(text string, at, n int) string {
	start := at
	for i := 0; i < snippetRunes/2 && start > 0; i++ {
		_, size := utf8.DecodeLastRuneInString(text[:start])
		start -= size
	}
	end := at + n
	for i := 0; i < snippetRunes/2 && end < len(text); i++ {
		_, size := utf8.DecodeRuneInString(text[end:])
		end += size
	}
	s := strings.Join(strings.Fields(text[start:end]), " ")
	if start > 0 {
		s = "…" + s
	}
	if end < len(text) {
		s += "…"
	}
	return s
}
