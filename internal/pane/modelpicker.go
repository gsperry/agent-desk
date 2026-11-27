package pane

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gsperry/agent-desk/internal/send"
)

// ModelRow is one model in Claude Code's "Select model" picker.
type ModelRow struct {
	Number      int
	Label       string
	Description string
	// Current marks the model the session runs now (a ✔ after its label).
	Current bool
}

// ModelPicker is what the picker shows. It lists three rows at a time and
// says how many more lie above and below; the highlighted row's effort
// level sits under the list.
type ModelPicker struct {
	Rows []ModelRow
	// Cursor indexes Rows.
	Cursor       int
	Above, Below int
	// Effort is the highlighted row's level label, such as "Medium", and
	// EffortDefault says the picker marks it "(default)".
	Effort        string
	EffortDefault bool
}

var (
	pickerTitle  = regexp.MustCompile(`^\s*Select model\s*$`)
	pickerRow    = regexp.MustCompile(`^\s*(❯)?\s*(\d{1,2})\.\s+(.+?)\s*$`)
	pickerMore   = regexp.MustCompile(`^\s*([↑↓])\s+(\d+) more\s*$`)
	pickerEffort = regexp.MustCompile(`^\s*\S\s+(\S+) effort(?: \((default)\))?\s+←/→ to adjust\s*$`)
	// pickerSession is the footer's offer to apply a choice to this session
	// only, which is the only way Agent Desk ever applies one: Enter would
	// save it as the user's default.
	pickerSession = regexp.MustCompile(`s to use this session only`)
	modelSet      = regexp.MustCompile(`Set model to (.+?) for this session only(?: with (\S+) effort)?\s*$`)
)

// effortOrder is the picker's slider from lowest to highest. Models can
// offer fewer levels, never others.
var effortOrder = []string{"Low", "Medium", "High", "xHigh", "Max"}

// ParsePicker reads Claude Code's model picker off a plain-text screen, or
// returns nil when the screen is not showing it. Only a picker that offers
// "s to use this session only" counts.
func ParsePicker(screen string) *ModelPicker {
	lines := strings.Split(screen, "\n")
	title := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if pickerTitle.MatchString(lines[i]) {
			title = i
			break
		}
	}
	if title < 0 {
		return nil
	}
	p := &ModelPicker{Cursor: -1}
	session := false
	for _, l := range lines[title+1:] {
		switch {
		case pickerSession.MatchString(l):
			session = true
		case pickerMore.MatchString(l):
			m := pickerMore.FindStringSubmatch(l)
			n, _ := strconv.Atoi(m[2])
			if m[1] == "↑" {
				p.Above = n
			} else {
				p.Below = n
			}
		case pickerEffort.MatchString(l):
			m := pickerEffort.FindStringSubmatch(l)
			p.Effort, p.EffortDefault = m[1], m[2] != ""
		default:
			if m := pickerRow.FindStringSubmatch(l); m != nil {
				n, _ := strconv.Atoi(m[2])
				row := ModelRow{Number: n}
				label, desc, _ := strings.Cut(gap.ReplaceAllString(m[3], "\x00"), "\x00")
				label = strings.TrimSpace(label)
				if strings.HasSuffix(label, "✔") {
					row.Current = true
					label = strings.TrimSpace(strings.TrimSuffix(label, "✔"))
				}
				row.Label, row.Description = label, strings.TrimSpace(desc)
				if m[1] != "" {
					p.Cursor = len(p.Rows)
				}
				p.Rows = append(p.Rows, row)
			}
		}
	}
	if !session || p.Cursor < 0 || p.Effort == "" {
		return nil
	}
	return p
}

// Total is how many models the picker lists, shown or hidden.
func (p *ModelPicker) Total() int { return len(p.Rows) + p.Above + p.Below }

// at is the highlighted row.
func (p *ModelPicker) at() ModelRow { return p.Rows[p.Cursor] }

// same reports two readings that show the same picker state.
func (p *ModelPicker) same(q *ModelPicker) bool {
	return q != nil && p.at() == q.at() && p.Effort == q.Effort && p.Above == q.Above && p.Below == q.Below
}

// ModelChoice is a model the picker offers, with the effort levels it
// takes, lowest first.
type ModelChoice struct {
	// Number is the row's place in the picker, which orders the list.
	Number        int      `json:"-"`
	Label         string   `json:"label"`
	Description   string   `json:"description,omitempty"`
	Current       bool     `json:"current,omitempty"`
	Efforts       []string `json:"efforts"`
	DefaultEffort string   `json:"defaultEffort,omitempty"`
}

// ModelOptions is everything the picker offers.
type ModelOptions struct {
	Models []ModelChoice `json:"models"`
}

// ModelResult is what a change came to: Applied is Claude Code's own
// words when it took the change, and Pending says it is asking the user to
// confirm a switch first.
type ModelResult struct {
	Applied string `json:"applied,omitempty"`
	Pending bool   `json:"pending,omitempty"`
}

const (
	reasonNotClaude send.Reason = "not_claude"
	reasonNotIdle   send.Reason = "not_idle"
	reasonPicker    send.Reason = "model_picker"
	reasonNoModel   send.Reason = "model_not_offered"
	reasonNoEffort  send.Reason = "effort_not_offered"
)

// maxPickerReads bounds how often a changing screen is read, and
// readPause is the wait between reads.
const (
	maxPickerReads = 12
	readPause      = 120 * time.Millisecond
)

func (a *Answerer) pause(ctx context.Context) {
	if a.Pause != nil {
		a.Pause(readPause)
		return
	}
	select {
	case <-ctx.Done():
	case <-time.After(readPause):
	}
}

// readPicker reads the screen and parses the picker, or nil.
func (a *Answerer) readPicker(ctx context.Context, id string) (*ModelPicker, string, error) {
	text, err := a.Reader.Read(ctx, id)
	if err != nil {
		return nil, "", fmt.Errorf("could not read the session's screen, so nothing was pressed: %w", err)
	}
	return ParsePicker(text), text, nil
}

// steady reads the picker until two readings in a row agree, since a key
// press lands a moment before the screen shows it.
func (a *Answerer) steady(ctx context.Context, id string) (*ModelPicker, error) {
	var prev *ModelPicker
	for i := 0; i < maxPickerReads; i++ {
		p, _, err := a.readPicker(ctx, id)
		if err != nil {
			return nil, err
		}
		if p == nil {
			return nil, &send.Refused{Reason: reasonPicker, Message: "Claude Code's model picker is no longer showing, so nothing more was pressed."}
		}
		if prev != nil && p.same(prev) {
			return p, nil
		}
		prev = p
		a.pause(ctx)
	}
	return nil, &send.Refused{Reason: reasonPicker, Message: "Claude Code's model picker kept changing, so nothing more was pressed."}
}

// claudeIdle finds the session and requires a running Claude Code session
// that is not working or asking.
func (a *Answerer) claudeIdle(id string) *send.Refused {
	if refused := a.check(id); refused != nil {
		return refused
	}
	for _, s := range a.Sessions.State().Sessions {
		if s.ID != id {
			continue
		}
		if s.Tool != "claude" {
			return &send.Refused{Reason: reasonNotClaude, Message: "Changing the model this way works only for Claude Code sessions."}
		}
		if s.Status != "idle" && s.Status != "finished" {
			return &send.Refused{Reason: reasonNotIdle, Message: "The session is " + s.Status + ". Change the model once it is idle at its prompt."}
		}
	}
	return nil
}

// openPicker types /model into an idle session's empty prompt and waits
// for the picker.
func (a *Answerer) openPicker(ctx context.Context, id string) (*ModelPicker, error) {
	if refused := a.claudeIdle(id); refused != nil {
		return nil, refused
	}
	if a.Typer == nil {
		return nil, &send.Refused{Reason: reasonBadText, Message: "Typed commands are not available."}
	}
	text, err := a.Reader.Read(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("could not read the session's screen, so nothing was typed: %w", err)
	}
	if refused := send.CheckPane(text); refused != nil {
		return nil, refused
	}
	if !send.AtPrompt(text) || Parse(text) != nil {
		return nil, &send.Refused{Reason: reasonNotIdle, Message: "The session is not at its empty prompt. Change the model once it is."}
	}
	if err := a.Typer.Type(ctx, id, "/model"); err != nil {
		return nil, err
	}
	for i := 0; i < maxPickerReads; i++ {
		p, _, err := a.readPicker(ctx, id)
		if err != nil {
			return nil, err
		}
		if p != nil {
			return a.steady(ctx, id)
		}
		a.pause(ctx)
	}
	return nil, &send.Refused{Reason: reasonPicker, Message: "Claude Code's model picker did not open."}
}

// closePicker presses Escape if, and only if, the picker is showing, which
// leaves the model as it was.
func (a *Answerer) closePicker(ctx context.Context, id string) {
	p, _, err := a.readPicker(ctx, id)
	if err != nil || p == nil {
		return
	}
	_ = a.Keys.Send(ctx, id, "Escape")
	for i := 0; i < 5; i++ {
		a.pause(ctx)
		if p, _, err = a.readPicker(ctx, id); err != nil || p == nil {
			return
		}
	}
}

// ModelOptions opens the picker, walks every model and its effort slider,
// and closes it again with Escape, so nothing changes.
func (a *Answerer) ModelOptions(ctx context.Context, id string) (ModelOptions, error) {
	p, err := a.openPicker(ctx, id)
	if err != nil {
		return ModelOptions{}, err
	}
	defer a.closePicker(ctx, id)
	total := p.Total()
	var out ModelOptions
	for step := 0; step < total; step++ {
		row := p.at()
		choice := ModelChoice{Number: row.Number, Label: row.Label, Description: row.Description, Current: row.Current}
		if _, choice.Efforts, choice.DefaultEffort, err = a.sweepEffort(ctx, id, p); err != nil {
			return ModelOptions{}, err
		}
		out.Models = append(out.Models, choice)
		if step == total-1 {
			break
		}
		if err := a.Keys.Send(ctx, id, "Down"); err != nil {
			return ModelOptions{}, err
		}
		if p, err = a.steady(ctx, id); err != nil {
			return ModelOptions{}, err
		}
		if p.at().Number == row.Number {
			return ModelOptions{}, &send.Refused{Reason: reasonPicker, Message: "Claude Code's model picker did not move, so its list could not be read."}
		}
	}
	// The walk starts at the current model and wraps; the list reads in the
	// picker's own order.
	sort.SliceStable(out.Models, func(i, j int) bool { return out.Models[i].Number < out.Models[j].Number })
	return out, nil
}

// sweepEffort learns the levels the highlighted model takes: Left to the
// lowest, then Right up to the highest, reading each.
func (a *Answerer) sweepEffort(ctx context.Context, id string, p *ModelPicker) (*ModelPicker, []string, string, error) {
	if err := a.Keys.Send(ctx, id, "Left", "Left", "Left", "Left", "Left", "Left"); err != nil {
		return p, nil, "", err
	}
	p, err := a.steady(ctx, id)
	if err != nil {
		return p, nil, "", err
	}
	var levels []string
	def := ""
	for i := 0; i < len(effortOrder)+1; i++ {
		levels = append(levels, p.Effort)
		if p.EffortDefault {
			def = p.Effort
		}
		if p.Effort == effortOrder[len(effortOrder)-1] {
			break
		}
		if err := a.Keys.Send(ctx, id, "Right"); err != nil {
			return p, nil, "", err
		}
		next, err := a.steady(ctx, id)
		if err != nil {
			return p, nil, "", err
		}
		if next.Effort == p.Effort {
			break
		}
		p = next
	}
	return p, levels, def, nil
}

// SetModel changes the session's model and, when effort is not empty, its
// effort, for this session only. It opens the picker, moves to the model
// and the level, checks the screen shows both, and presses s. It never
// presses Enter in the picker, which would save the choice as the user's
// default. If Claude Code asks to confirm a switch, the question is left
// for the user.
func (a *Answerer) SetModel(ctx context.Context, id, model, effort string) (ModelResult, error) {
	p, err := a.openPicker(ctx, id)
	if err != nil {
		return ModelResult{}, err
	}
	fail := func(refused error) (ModelResult, error) {
		a.closePicker(ctx, id)
		return ModelResult{}, refused
	}
	for step := 0; p.at().Label != model; step++ {
		if step >= p.Total() {
			return fail(&send.Refused{Reason: reasonNoModel, Message: fmt.Sprintf("Claude Code's picker does not offer %q.", model)})
		}
		if err := a.Keys.Send(ctx, id, "Down"); err != nil {
			return fail(err)
		}
		if p, err = a.steady(ctx, id); err != nil {
			return ModelResult{}, err
		}
	}
	if effort != "" {
		for step := 0; p.Effort != effort; step++ {
			from, to := effortIndex(p.Effort), effortIndex(effort)
			if step > len(effortOrder) || from < 0 || to < 0 {
				return fail(&send.Refused{Reason: reasonNoEffort, Message: fmt.Sprintf("%s does not offer %s effort.", model, effort)})
			}
			key := "Right"
			if to < from {
				key = "Left"
			}
			if err := a.Keys.Send(ctx, id, key); err != nil {
				return fail(err)
			}
			next, err := a.steady(ctx, id)
			if err != nil {
				return ModelResult{}, err
			}
			if next.Effort == p.Effort {
				return fail(&send.Refused{Reason: reasonNoEffort, Message: fmt.Sprintf("%s does not offer %s effort.", model, effort)})
			}
			p = next
		}
	}
	if p.at().Label != model || (effort != "" && p.Effort != effort) {
		return fail(&send.Refused{Reason: reasonPicker, Message: "The picker did not show the choice, so nothing was applied."})
	}
	// The screen already shows earlier "Set model" lines; only a newer one
	// is this change.
	before, _, err := a.beforeApply(ctx, id)
	if err != nil {
		return fail(err)
	}
	if err := a.Keys.Send(ctx, id, "s"); err != nil {
		return fail(err)
	}
	return a.afterApply(ctx, id, model, effort, before)
}

type setMark struct {
	count int
	last  string
}

func (a *Answerer) beforeApply(ctx context.Context, id string) (setMark, string, error) {
	text, err := a.Reader.Read(ctx, id)
	if err != nil {
		return setMark{}, "", fmt.Errorf("could not read the session's screen, so nothing was pressed: %w", err)
	}
	return markOf(text), text, nil
}

func markOf(text string) setMark {
	m := setMark{}
	for _, l := range strings.Split(text, "\n") {
		if line, ok := parseModelSet(l); ok {
			m.count++
			m.last = line.text
		}
	}
	return m
}

// afterApply reads what Claude Code did with the choice.
func (a *Answerer) afterApply(ctx context.Context, id, model, effort string, before setMark) (ModelResult, error) {
	for i := 0; i < maxPickerReads; i++ {
		a.pause(ctx)
		text, err := a.Reader.Read(ctx, id)
		if err != nil {
			return ModelResult{}, fmt.Errorf("could not read the session's screen after applying: %w", err)
		}
		if ParsePicker(text) != nil {
			continue
		}
		if d := Parse(text); d != nil && strings.Contains(d.Question, "Switch model?") {
			return ModelResult{Pending: true}, nil
		}
		now := markOf(text)
		if now.count == 0 || (now.count == before.count && now.last == before.last) {
			continue
		}
		line, _ := lastModelSet(text)
		// The Default row names whatever model it stands for.
		named := model == "Default" || strings.HasPrefix(line.model, model)
		if named && (effort == "" || line.effort == "" || strings.EqualFold(line.effort, effort)) {
			return ModelResult{Applied: line.text}, nil
		}
	}
	return ModelResult{}, &send.Refused{Reason: reasonPicker, Message: "The choice was sent, but the screen did not confirm it. Check the session in agent-manager."}
}

type setLine struct {
	text, model, effort string
}

// lastModelSet is the newest "Set model to … for this session only" line
// on the screen.
func lastModelSet(text string) (setLine, bool) {
	var out setLine
	found := false
	for _, l := range strings.Split(text, "\n") {
		if line, ok := parseModelSet(l); ok {
			out, found = line, true
		}
	}
	return out, found
}

func parseModelSet(l string) (setLine, bool) {
	l = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "⎿"))
	m := modelSet.FindStringSubmatch(l)
	if m == nil {
		return setLine{}, false
	}
	return setLine{text: l, model: m[1], effort: m[2]}, true
}

func effortIndex(level string) int {
	for i, l := range effortOrder {
		if l == level {
			return i
		}
	}
	return -1
}
