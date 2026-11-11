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

	// refreshing lets one refresh run at a time; a second Report waits for
	// it and then finds the files already read.
	refreshing sync.Mutex
	mu         sync.Mutex
	files      map[string]*fileState
}

// fileState is what a file has contributed so far, keyed by record, plus
// what is needed to carry on reading it. Contributions are kept per record
// so a reply copied into a resumed conversation's file counts once.
type fileState struct {
	path     string
	conv     string
	subagent bool
	size     int64
	modTime  time.Time
	offset   int64
	parser   *transcript.Parser
	messages map[string]*message
	prompts  map[string]*prompt
	costs    map[string]float64
	first    time.Time
	last     time.Time
	// Turn timing for main transcripts.
	lastPrompt    time.Time
	lastEvent     time.Time
	lastAssistant time.Time
	// Claude Code's cost counter restarts with each process.
	lastCost  float64
	costStart string
}

// message is one model reply. Claude Code writes a reply as a record per
// content block, each with the usage so far, so tokens keep the largest.
type message struct {
	day, model string
	tokens     Tokens
	workingMs  int64
}

type prompt struct {
	day       string
	waitingMs int64
}

func newFileState(t target) *fileState {
	return &fileState{
		path: t.path, conv: t.conv, subagent: t.subagent,
		parser:   transcript.NewParser(),
		messages: map[string]*message{},
		prompts:  map[string]*prompt{},
		costs:    map[string]float64{},
	}
}

// clone copies a state so a read in progress never changes what a
// concurrent Report sees, and a failed read leaves the saved state as it was.
func (f *fileState) clone() *fileState {
	c := *f
	c.parser = f.parser.Clone()
	c.messages = make(map[string]*message, len(f.messages))
	for k, v := range f.messages {
		m := *v
		c.messages[k] = &m
	}
	c.prompts = make(map[string]*prompt, len(f.prompts))
	for k, v := range f.prompts {
		p := *v
		c.prompts[k] = &p
	}
	c.costs = make(map[string]float64, len(f.costs))
	for k, v := range f.costs {
		c.costs[k] = v
	}
	return &c
}

const workers = 4

// Report totals the last days days, today included. A reply or prompt that
// appears in more than one file counts once, for the conversation that
// started first.
func (s *Scanner) Report(ctx context.Context, days int) (Report, error) {
	if days < 1 {
		days = 30
	}
	s.refreshing.Lock()
	paths, err := s.paths()
	if err == nil {
		err = s.refresh(ctx, paths)
	}
	s.refreshing.Unlock()
	if err != nil {
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
	files := make([]*fileState, 0, len(s.files))
	for _, f := range s.files {
		files = append(files, f)
	}
	s.mu.Unlock()
	sort.Slice(files, func(i, j int) bool {
		a, b := files[i], files[j]
		if !a.first.Equal(b.first) {
			if a.first.IsZero() || b.first.IsZero() {
				return b.first.IsZero()
			}
			return a.first.Before(b.first)
		}
		// A resumed conversation starts with copies of the original's
		// records, so both start together; the original ends first.
		if !a.last.Equal(b.last) {
			return a.last.Before(b.last)
		}
		return a.path < b.path
	})

	rep := Report{From: from, To: to, Files: len(files)}
	byDay := map[string]*Bucket{}
	byModel := map[string]*Tokens{}
	byConv := map[string]*Conversation{}
	seenMessages := map[string]bool{}
	seenPrompts := map[string]bool{}
	inRange := func(day string) bool { return day >= from && day <= to }
	add := func(f *fileState, day string, b Bucket) {
		rep.Totals.add(b)
		if byDay[day] == nil {
			byDay[day] = &Bucket{}
		}
		byDay[day].add(b)
		c := byConv[f.conv]
		if c == nil {
			c = &Conversation{ID: f.conv}
			byConv[f.conv] = c
		}
		c.add(b)
		if f.subagent {
			c.SubagentTokens += b.Total()
		}
		if l := f.last.UTC().Format(time.RFC3339); !f.last.IsZero() && l > c.Last {
			c.Last = l
		}
	}
	for _, f := range files {
		for id, m := range f.messages {
			if seenMessages[id] {
				continue
			}
			seenMessages[id] = true
			if !inRange(m.day) {
				continue
			}
			add(f, m.day, Bucket{Tokens: m.tokens, Replies: 1, WorkingMs: m.workingMs})
			if byModel[m.model] == nil {
				byModel[m.model] = &Tokens{}
			}
			byModel[m.model].add(m.tokens)
		}
		for id, p := range f.prompts {
			if seenPrompts[id] {
				continue
			}
			seenPrompts[id] = true
			if inRange(p.day) {
				add(f, p.day, Bucket{Prompts: 1, WaitingMs: p.waitingMs})
			}
		}
		for day, usd := range f.costs {
			if inRange(day) && usd != 0 {
				add(f, day, Bucket{CostUSD: usd})
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
	sort.Slice(rep.Models, func(i, j int) bool {
		if rep.Models[i].Total() != rep.Models[j].Total() {
			return rep.Models[i].Total() > rep.Models[j].Total()
		}
		return rep.Models[i].Model < rep.Models[j].Model
	})
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
send:
	for _, t := range targets {
		if ctx.Err() != nil {
			break
		}
		select {
		case jobs <- t:
		case <-ctx.Done():
			break send
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
		f = newFileState(t)
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
	UUID      string `json:"uuid"`
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

func (s *Scanner) day(at time.Time) string {
	return at.In(s.loc()).Format(time.DateOnly)
}

// seen widens the file's time span.
func (f *fileState) seen(at time.Time) {
	if f.first.IsZero() || at.Before(f.first) {
		f.first = at
	}
	if at.After(f.last) {
		f.last = at
	}
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
	key := rec.Message.ID
	if key == "" {
		if rec.UUID == "" {
			return
		}
		key = "uuid:" + rec.UUID
	}
	u := rec.Message.Usage
	tok := Tokens{Input: u.Input, Output: u.Output, CacheRead: u.CacheRead, CacheWrite: u.CacheWrite}
	m := f.messages[key]
	if m == nil {
		model := rec.Message.Model
		if model == "" || model == "<synthetic>" {
			model = "unknown"
		}
		m = &message{day: s.day(at), model: model, tokens: tok}
		f.messages[key] = m
	} else {
		m.tokens = Tokens{max(tok.Input, m.tokens.Input), max(tok.Output, m.tokens.Output), max(tok.CacheRead, m.tokens.CacheRead), max(tok.CacheWrite, m.tokens.CacheWrite)}
	}
	if !f.subagent {
		if !f.lastEvent.IsZero() && at.After(f.lastEvent) {
			if gap := at.Sub(f.lastEvent); gap <= workGap {
				m.workingMs += gap.Milliseconds()
			}
		}
		if at.After(f.lastEvent) {
			f.lastEvent = at
		}
		f.lastAssistant = at
	}
	f.seen(at)
}

func (s *Scanner) user(f *fileState, line []byte) {
	if f.subagent {
		return
	}
	for _, it := range f.parser.Line(line) {
		if it.Kind != transcript.KindUser {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, it.Time)
		if err != nil {
			continue
		}
		key := it.UUID
		if key == "" {
			key = "time:" + it.Time
		}
		if f.prompts[key] != nil {
			continue
		}
		p := &prompt{day: s.day(at)}
		if !f.lastAssistant.IsZero() && f.lastAssistant.After(f.lastPrompt) && at.After(f.lastAssistant) {
			p.waitingMs = min(at.Sub(f.lastAssistant), waitCap).Milliseconds()
		}
		f.prompts[key] = p
		f.lastPrompt, f.lastEvent = at, at
		f.seen(at)
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
	f.costs[s.day(f.last)] += delta
}
