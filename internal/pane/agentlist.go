package pane

import (
	"regexp"
	"strconv"
	"strings"
)

// AgentList is Claude Code's list of a session's agents, drawn under the
// prompt while subagents run: "main" first, then each subagent. Down from
// an empty prompt selects in it ("↑/↓ to select"), Enter views the
// selected agent, and x stops a running one. Viewing a subagent turns the
// prompt into a message box for it ("Message @<type>…") under a rule
// titled with its description. Esc while viewing interrupts that subagent,
// so Agent Desk never presses Escape there.
type AgentList struct {
	Rows []AgentRow
	// Hint is the line above the rows: the prompt's own hints, or the
	// selection hints ("↑/↓ to select · Enter to view · x to stop").
	Hint string
	// Selecting is set while the list takes Up, Down and Enter.
	Selecting bool
	// Cursor is the selected row while Selecting, else -1.
	Cursor int
	// Shown is the row whose conversation the screen shows (⏺).
	Shown int
	// Viewing is set when the screen shows a subagent's conversation; its
	// prompt then messages that subagent.
	Viewing bool
	// Title is the title drawn into the rule above the prompt: the viewed
	// subagent's description, or the session's own title.
	Title string
	// PromptEmpty reports an empty prompt, or one showing only its
	// placeholder.
	PromptEmpty bool
}

// AgentRow is one agent in the list.
type AgentRow struct {
	Main bool
	// Type is the agent's type, such as general-purpose.
	Type string
	// Text is its description, or the step it is on.
	Text string
	// Stats is the elapsed time and tokens, and how many messages wait.
	Stats  string
	Queued int
}

var (
	agentRowLine = regexp.MustCompile(`^(❯[ \x{00a0}])?\s*([⏺◯])[\s\x{00a0}]+(\S.*?)\s*$`)
	wideGap      = regexp.MustCompile(`\s{2,}`)
	queuedCount  = regexp.MustCompile(`(\d+) queued`)
	// Claude Code puts a no-break space after the prompt's ❯.
	placeholder = regexp.MustCompile(`^❯[\s\x{00a0}]+Message @\S+…\s*$`)
)

// ParseAgents reads the agent list at the bottom of a screen, or returns
// nil when there is none.
func ParseAgents(screen string) *AgentList {
	var lines []string
	for _, l := range strings.Split(screen, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	end := len(lines)
	start := end
	for start > 0 && agentRowLine.MatchString(lines[start-1]) && !isRule(lines[start-1]) {
		start--
	}
	// The hint line, the prompt's bottom rule, the prompt and its top rule
	// sit above the rows.
	if start == end || start < 4 {
		return nil
	}
	hint, bottom, prompt, top := lines[start-1], lines[start-2], lines[start-3], lines[start-4]
	if !isRule(bottom) || !isRule(top) || !strings.HasPrefix(strings.TrimSpace(prompt), "❯") {
		return nil
	}
	l := &AgentList{Hint: strings.TrimSpace(hint), Cursor: -1, Shown: -1}
	for i, line := range lines[start:end] {
		m := agentRowLine.FindStringSubmatch(line)
		row := AgentRow{}
		parts := wideGap.Split(m[3], -1)
		if len(parts) == 1 && parts[0] == "main" {
			row.Main = true
		} else {
			row.Type = parts[0]
			if len(parts) > 1 {
				row.Text = parts[1]
			}
			if len(parts) > 2 {
				row.Stats = strings.Join(parts[2:], " ")
			}
			if q := queuedCount.FindStringSubmatch(row.Stats); q != nil {
				row.Queued, _ = strconv.Atoi(q[1])
			}
		}
		if m[1] != "" {
			l.Cursor = i
		}
		if m[2] == "⏺" {
			l.Shown = i
		}
		l.Rows = append(l.Rows, row)
	}
	if len(l.Rows) < 2 || !l.Rows[0].Main {
		return nil
	}
	l.Selecting = l.Cursor >= 0
	p := strings.TrimSpace(prompt)
	l.Viewing = placeholder.MatchString(p)
	l.PromptEmpty = p == "❯" || l.Viewing
	l.Title = ruleTitle(top)
	return l
}

// ruleTitle is the text drawn into a rule ("──── Pinger ─" is Pinger).
func ruleTitle(l string) string {
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(l), "─"))
}

// Matches reports whether a row is the subagent of this type and
// description. A row shows the description or, while the subagent works,
// the step it is on, so a row that matches by type is only a candidate
// until viewing it shows the description in the prompt's rule. A
// plugin-qualified type may show without its plugin.
func (r AgentRow) matchesType(agentType string) bool {
	if r.Main {
		return false
	}
	if r.Type == agentType {
		return true
	}
	if i := strings.LastIndex(agentType, ":"); i >= 0 && r.Type == agentType[i+1:] {
		return true
	}
	return false
}

// candidates are the rows that may be the subagent: those of its type,
// with one already showing its description first.
func (l *AgentList) candidates(agentType, description string) []int {
	var exact, rest []int
	for i, r := range l.Rows {
		if !r.matchesType(agentType) {
			continue
		}
		if r.Text == description {
			exact = append(exact, i)
		} else {
			rest = append(rest, i)
		}
	}
	return append(exact, rest...)
}
