// Package usage totals tokens, time and reported cost from Claude Code
// transcripts, by day, model and conversation. Subagent transcripts count
// toward the conversation that spawned them.
//
// Files are read once and then followed from where the last read stopped,
// so a refresh only reads what Claude Code appended.
package usage

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gsperry/agent-desk/internal/transcript"
)

// Tokens counts what the model read and wrote.
type Tokens struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheRead  int64 `json:"cacheRead"`
	CacheWrite int64 `json:"cacheWrite"`
}

func (t *Tokens) add(o Tokens) {
	t.Input += o.Input
	t.Output += o.Output
	t.CacheRead += o.CacheRead
	t.CacheWrite += o.CacheWrite
}

// Total is every token the model processed.
func (t Tokens) Total() int64 { return t.Input + t.Output + t.CacheRead + t.CacheWrite }

// Bucket is one day of one conversation, or a sum of them.
type Bucket struct {
	Tokens
	Replies int `json:"replies"`
	Prompts int `json:"prompts"`
	// WorkingMs is time from a prompt through the agent's replies, leaving
	// out pauses longer than workGap.
	WorkingMs int64 `json:"workingMs"`
	// WaitingMs is time from the agent's last reply to the next prompt,
	// each wait capped at waitCap.
	WaitingMs int64 `json:"waitingMs"`
	// CostUSD is what Claude Code reported, at list price.
	CostUSD float64 `json:"costUSD"`
}

func (b *Bucket) add(o Bucket) {
	b.Tokens.add(o.Tokens)
	b.Replies += o.Replies
	b.Prompts += o.Prompts
	b.WorkingMs += o.WorkingMs
	b.WaitingMs += o.WaitingMs
	b.CostUSD += o.CostUSD
}

const (
	workGap = 5 * time.Minute
	waitCap = time.Hour
)

// Day is one calendar day in local time.
type Day struct {
	Day string `json:"day"`
	Bucket
}

type Model struct {
	Model string `json:"model"`
	Tokens
}

type Conversation struct {
	ID string `json:"id"`
	Bucket
	SubagentTokens int64  `json:"subagentTokens"`
	Last           string `json:"last,omitempty"`
}

type Report struct {
	From          string         `json:"from"`
	To            string         `json:"to"`
	Totals        Bucket         `json:"totals"`
	Days          []Day          `json:"days"`
	Models        []Model        `json:"models"`
	Conversations []Conversation `json:"conversations"`
	Files         int            `json:"files"`
}

// Scanner reads transcripts under Root (a Claude config directory).
type Scanner struct {
	Root string
	// Now and Location default to time.Now and time.Local.
	Now      func() time.Time
	Location *time.Location

	mu    sync.Mutex
	files map[string]*fileState
}

// fileState is what a file has contributed so far, plus what is needed to
// carry on reading it.
type fileState struct {
	conv     string
	subagent bool
	size     int64
	modTime  time.Time
	offset   int64
	parser   *transcript.Parser
	messages map[string]seenMessage
	days     map[string]*dayState
	// Turn timing for main transcripts.
	lastPrompt    time.Time
	lastEvent     time.Time
	lastAssistant time.Time
	// Claude Code's cost counter restarts with each process.
	lastCost  float64
	costStart string
	last      time.Time
}

type dayState struct {
	Bucket
	models map[string]*Tokens
}

type seenMessage struct {
	day, model string
	tokens     Tokens
}

const workers = 4

// Report totals the last days days, today included.
func (s *Scanner) Report(ctx context.Context, days int) (Report, error) {
	if days < 1 {
		days = 30
	}
	paths, err := s.paths()
	if err != nil {
		return Report{}, err
	}
	if err := s.refresh(ctx, paths); err != nil {
		return Report{}, err
	}
	loc := s.loc()
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	to := now.In(loc).Format(time.DateOnly)
	from := now.In(loc).AddDate(0, 0, -(days - 1)).Format(time.DateOnly)

	s.mu.Lock()
	defer s.mu.Unlock()
	rep := Report{From: from, To: to, Files: len(s.files)}
	byDay := map[string]*Bucket{}
	byModel := map[string]*Tokens{}
	byConv := map[string]*Conversation{}
	for _, f := range s.files {
		for day, d := range f.days {
			if day < from || day > to {
				continue
			}
			rep.Totals.add(d.Bucket)
			if byDay[day] == nil {
				byDay[day] = &Bucket{}
			}
			byDay[day].add(d.Bucket)
			for m, t := range d.models {
				if byModel[m] == nil {
					byModel[m] = &Tokens{}
				}
				byModel[m].add(*t)
			}
			c := byConv[f.conv]
			if c == nil {
				c = &Conversation{ID: f.conv}
				byConv[f.conv] = c
			}
			c.add(d.Bucket)
			if f.subagent {
				c.SubagentTokens += d.Total()
			}
			if l := f.last.UTC().Format(time.RFC3339); !f.last.IsZero() && l > c.Last {
				c.Last = l
			}
		}
	}
	for day := from; day <= to; day = nextDay(day) {
		b := Bucket{}
		if byDay[day] != nil {
			b = *byDay[day]
		}
		rep.Days = append(rep.Days, Day{Day: day, Bucket: b})
	}
	for m, t := range byModel {
		rep.Models = append(rep.Models, Model{Model: m, Tokens: *t})
	}
	sort.Slice(rep.Models, func(i, j int) bool { return rep.Models[i].Total() > rep.Models[j].Total() })
	for _, c := range byConv {
		rep.Conversations = append(rep.Conversations, *c)
	}
	sort.Slice(rep.Conversations, func(i, j int) bool {
		a, b := rep.Conversations[i], rep.Conversations[j]
		if a.Total() != b.Total() {
			return a.Total() > b.Total()
		}
		return a.ID < b.ID
	})
	return rep, nil
}

func nextDay(day string) string {
	t, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return "9999-12-31"
	}
	return t.AddDate(0, 0, 1).Format(time.DateOnly)
}

func (s *Scanner) loc() *time.Location {
	if s.Location != nil {
		return s.Location
	}
	return time.Local
}

type target struct {
	path, conv string
	subagent   bool
}

func (s *Scanner) paths() ([]target, error) {
	var out []target
	main, err := filepath.Glob(filepath.Join(s.Root, "projects", "*", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	for _, p := range main {
		out = append(out, target{path: p, conv: strings.TrimSuffix(filepath.Base(p), ".jsonl")})
	}
	subs, err := filepath.Glob(filepath.Join(s.Root, "projects", "*", "*", "subagents", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	for _, p := range subs {
		conv := filepath.Base(filepath.Dir(filepath.Dir(p)))
		out = append(out, target{path: p, conv: conv, subagent: true})
	}
	return out, nil
}

func (s *Scanner) refresh(ctx context.Context, targets []target) error {
	s.mu.Lock()
	if s.files == nil {
		s.files = map[string]*fileState{}
	}
	live := map[string]bool{}
	for _, t := range targets {
		live[t.path] = true
	}
	for p := range s.files {
		if !live[p] {
			delete(s.files, p)
		}
	}
	s.mu.Unlock()

	jobs := make(chan target)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range jobs {
				s.refreshFile(t)
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
	return ctx.Err()
}

func (s *Scanner) refreshFile(t target) {
	info, err := os.Stat(t.path)
	if err != nil || !info.Mode().IsRegular() {
		s.mu.Lock()
		delete(s.files, t.path)
		s.mu.Unlock()
		return
	}
	s.mu.Lock()
	f := s.files[t.path]
	s.mu.Unlock()
	if f != nil && f.size == info.Size() && f.modTime.Equal(info.ModTime()) {
		return
	}
	// A file that shrank was rewritten; read it again from the start.
	if f == nil || info.Size() < f.offset {
		f = &fileState{conv: t.conv, subagent: t.subagent, parser: transcript.NewParser(), messages: map[string]seenMessage{}, days: map[string]*dayState{}}
	} else {
		f = f.clone()
	}
	if err := s.read(t.path, f); err != nil {
		return
	}
	f.size, f.modTime = info.Size(), info.ModTime()
	s.mu.Lock()
	s.files[t.path] = f
	s.mu.Unlock()
}

// clone copies a state so a read in progress never changes what a
// concurrent Report sees. Maps are copied shallowly where values are
// replaced, not mutated.
func (f *fileState) clone() *fileState {
	c := *f
	c.messages = make(map[string]seenMessage, len(f.messages))
	for k, v := range f.messages {
		c.messages[k] = v
	}
	c.days = make(map[string]*dayState, len(f.days))
	for k, v := range f.days {
		d := &dayState{Bucket: v.Bucket, models: make(map[string]*Tokens, len(v.models))}
		for m, t := range v.models {
			tt := *t
			d.models[m] = &tt
		}
		c.days[k] = d
	}
	return &c
}

func (s *Scanner) read(path string, f *fileState) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Seek(f.offset, io.SeekStart); err != nil {
		return err
	}
	r := bufio.NewReaderSize(file, 1<<20)
	for {
		line, err := r.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			// A last line without a newline may still be being written;
			// it is read on the next refresh.
			return nil
		}
		if err != nil {
			return err
		}
		f.offset += int64(len(line))
		s.line(f, line)
	}
}

var (
	markAssistant = []byte(`"type":"assistant"`)
	markUsage     = []byte(`"usage"`)
	markUser      = []byte(`"type":"user"`)
	markCost      = []byte(`"type":"cost-state"`)
)

type assistantRecord struct {
	Timestamp string `json:"timestamp"`
	Message   struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage *struct {
			Input      int64 `json:"input_tokens"`
			Output     int64 `json:"output_tokens"`
			CacheRead  int64 `json:"cache_read_input_tokens"`
			CacheWrite int64 `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// costRecord is Claude Code's running cost for one process, identified by
// its start time. It has no timestamp of its own.
type costRecord struct {
	StartTime    json.RawMessage `json:"startTime"`
	TotalCostUSD *float64        `json:"totalCostUSD"`
}

func (s *Scanner) line(f *fileState, line []byte) {
	switch {
	case bytes.Contains(line, markAssistant) && bytes.Contains(line, markUsage):
		s.assistant(f, line)
	case bytes.Contains(line, markUser):
		s.user(f, line)
	case bytes.Contains(line, markCost):
		s.cost(f, line)
	}
}

func (s *Scanner) day(f *fileState, at time.Time) *dayState {
	key := at.In(s.loc()).Format(time.DateOnly)
	d := f.days[key]
	if d == nil {
		d = &dayState{models: map[string]*Tokens{}}
		f.days[key] = d
	}
	return d
}

func (s *Scanner) assistant(f *fileState, line []byte) {
	var rec assistantRecord
	if json.Unmarshal(line, &rec) != nil || rec.Message.Usage == nil {
		return
	}
	at, err := time.Parse(time.RFC3339Nano, rec.Timestamp)
	if err != nil {
		return
	}
	u := rec.Message.Usage
	tok := Tokens{Input: u.Input, Output: u.Output, CacheRead: u.CacheRead, CacheWrite: u.CacheWrite}
	model := rec.Message.Model
	if model == "" || model == "<synthetic>" {
		model = "unknown"
	}
	d := s.day(f, at)
	if prev, ok := f.messages[rec.Message.ID]; ok && rec.Message.ID != "" {
		// One reply is written as a record per content block, each with the
		// usage so far. Count only what grew.
		delta := Tokens{
			Input:      max(tok.Input-prev.tokens.Input, 0),
			Output:     max(tok.Output-prev.tokens.Output, 0),
			CacheRead:  max(tok.CacheRead-prev.tokens.CacheRead, 0),
			CacheWrite: max(tok.CacheWrite-prev.tokens.CacheWrite, 0),
		}
		pd := f.days[prev.day]
		pd.Tokens.add(delta)
		pd.models[prev.model].add(delta)
		prev.tokens = Tokens{max(tok.Input, prev.tokens.Input), max(tok.Output, prev.tokens.Output), max(tok.CacheRead, prev.tokens.CacheRead), max(tok.CacheWrite, prev.tokens.CacheWrite)}
		f.messages[rec.Message.ID] = prev
	} else {
		d.Tokens.add(tok)
		d.Replies++
		if d.models[model] == nil {
			d.models[model] = &Tokens{}
		}
		d.models[model].add(tok)
		if rec.Message.ID != "" {
			f.messages[rec.Message.ID] = seenMessage{day: at.In(s.loc()).Format(time.DateOnly), model: model, tokens: tok}
		}
	}
	if !f.subagent {
		if !f.lastEvent.IsZero() && at.After(f.lastEvent) {
			if gap := at.Sub(f.lastEvent); gap <= workGap {
				d.WorkingMs += gap.Milliseconds()
			}
		}
		if at.After(f.lastEvent) {
			f.lastEvent = at
		}
		f.lastAssistant = at
	}
	if at.After(f.last) {
		f.last = at
	}
}

func (s *Scanner) user(f *fileState, line []byte) {
	if f.subagent {
		return
	}
	var at time.Time
	prompt := false
	for _, it := range f.parser.Line(line) {
		if it.Kind != transcript.KindUser {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, it.Time)
		if err != nil {
			continue
		}
		at, prompt = t, true
	}
	if !prompt {
		return
	}
	d := s.day(f, at)
	d.Prompts++
	if !f.lastAssistant.IsZero() && f.lastAssistant.After(f.lastPrompt) && at.After(f.lastAssistant) {
		d.WaitingMs += min(at.Sub(f.lastAssistant), waitCap).Milliseconds()
	}
	f.lastPrompt, f.lastEvent = at, at
	if at.After(f.last) {
		f.last = at
	}
}

func (s *Scanner) cost(f *fileState, line []byte) {
	var rec costRecord
	if json.Unmarshal(line, &rec) != nil || rec.TotalCostUSD == nil {
		return
	}
	// The cost counts toward the day of the latest activity before it.
	if f.last.IsZero() {
		return
	}
	v := *rec.TotalCostUSD
	delta := v - f.lastCost
	if start := string(rec.StartTime); start != f.costStart || delta < 0 {
		// A new Claude Code process counts from zero.
		delta = v
		f.costStart = start
	}
	f.lastCost = v
	s.day(f, f.last).CostUSD += delta
}
