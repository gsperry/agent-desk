// Package pane reads a dialog off an agent's screen and answers it with
// key presses. It is the one place Agent Desk drives the terminal UI
// directly, because agent-manager has no command for it yet
// (docs/upstream/README.md).
package pane

import (
	"regexp"
	"strings"
	"unicode"
)

// Dialog is a choice the agent's terminal is waiting on.
type Dialog struct {
	Question string   `json:"question,omitempty"`
	Options  []Option `json:"options"`
	Footer   string   `json:"footer"`
	// Multi is a question that takes several answers: its options are
	// checkboxes that Enter ticks, and Next moves on.
	Multi bool `json:"multi,omitempty"`
	// Steps are the tabs of a prompt with several questions, in order.
	Steps []Step `json:"steps,omitempty"`
}

// Step is one tab of a prompt with several questions. Done marks a
// question with an answer; Submit is the final review step. The plain
// screen does not show which tab is current (the terminal highlights it),
// so neither does a Step.
type Step struct {
	Label  string `json:"label"`
	Done   bool   `json:"done,omitempty"`
	Submit bool   `json:"submit,omitempty"`
}

// Option is one entry in the dialog's list. Index is its position in the
// list as the arrow keys walk it, which is what answers are given in.
type Option struct {
	Index    int    `json:"index"`
	Label    string `json:"label"`
	Selected bool   `json:"selected"`
	// Number is the number the screen shows, if any; Next has none.
	Number int `json:"number,omitempty"`
	// Free options are a text field in the dialog ("Type something."):
	// they are answered with text, not a key press.
	Free bool `json:"free,omitempty"`
	// Checked is set on a checkbox option, true when it is ticked.
	Checked *bool `json:"checked,omitempty"`
	// Next moves a several-answer question on to its next step.
	Next bool `json:"next,omitempty"`
	// Description is the dimmer text under the label, when there is one.
	Description string `json:"description,omitempty"`
}

var (
	footerPattern = regexp.MustCompile(`Enter to (?:confirm|select)|Esc to cancel`)
	numbered      = regexp.MustCompile(`^(\s*)(❯)?\s*(\d{1,2})\.\s+(.+?)\s*$`)
	separator     = regexp.MustCompile(`^\s*[─╌━-]{10,}\s*$`)
	// freeLabel is an option that is a text field. "Chat about this" is not
	// one: choosing it declines the question and returns to the prompt.
	freeLabel = regexp.MustCompile(`^Type something`)
	checkbox  = regexp.MustCompile(`^\[(.)\]\s+(.+)$`)
	nextLine  = regexp.MustCompile(`^\s*(❯)?\s*Next\s*$`)
	stepItem  = regexp.MustCompile(`^([☐☒✔])\s+(.+)$`)
	gap       = regexp.MustCompile(`\s{2,}`)
)

// footerLines is how far up from the bottom the footer is looked for, the
// same window the send gate uses.
const footerLines = 8

// maxScan bounds how far above the footer options are looked for.
const maxScan = 40

// Parse finds a dialog on a plain-text screen, or returns nil.
func Parse(screen string) *Dialog {
	lines, gaps := nonBlank(screen)
	footer := -1
	for i := len(lines) - 1; i >= max(len(lines)-footerLines, 0); i-- {
		if footerPattern.MatchString(lines[i]) {
			footer = i
			break
		}
	}
	footerText := ""
	if footer >= 0 {
		footerText = strings.TrimSpace(lines[footer])
	} else if reviewScreen(lines) {
		// Claude Code's last step of a multi-question prompt has no
		// footer; the options end the screen.
		footer = len(lines)
	} else {
		return nil
	}
	top := max(footer-maxScan, 0)
	d := &Dialog{Footer: footerText}
	first := numberedOptions(lines, top, footer, d)
	if first < 0 {
		first = markedOptions(lines, top, footer, d)
	}
	if first < 0 {
		return d
	}
	d.Question = question(lines, gaps, top, first, screenWidth(lines))
	d.Steps = steps(lines, top, first)
	return d
}

// reviewPrompt is the question on Claude Code's review step for a prompt
// with several questions.
var reviewPrompt = regexp.MustCompile(`^Ready to submit your answers\?$`)

// reviewScreen reports a review step: the prompt line just above the
// numbered options that end the screen.
func reviewScreen(lines []string) bool {
	for i := len(lines) - 1; i >= max(len(lines)-footerLines, 0); i-- {
		l := strings.TrimSpace(lines[i])
		if reviewPrompt.MatchString(l) {
			return true
		}
		// A rule can close the screen below the options.
		if isRule(lines[i]) {
			continue
		}
		if !numbered.MatchString(lines[i]) {
			return false
		}
	}
	return false
}

// numberedOptions walks up from the footer collecting "N. label" lines down
// to 1, skipping descriptions and separators between them. It returns the
// line of option 1, or -1.
func numberedOptions(lines []string, top, footer int, d *Dialog) int {
	want := 0
	var opts []Option
	var lineOf []int
	first := -1
	for i := footer - 1; i >= top; i-- {
		m := numbered.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		n := atoi(m[3])
		if want != 0 && n != want-1 {
			break
		}
		o := option(n-1, m[4], m[2] != "")
		o.Number = n
		opts = append(opts, o)
		lineOf = append(lineOf, i)
		want, first = n, i
		if n == 1 {
			break
		}
	}
	if want != 1 {
		return -1
	}
	// A several-answer question lists Next, unnumbered, among the options;
	// it takes its place in the arrow-key order.
	next := -1
	for i := first + 1; i < footer; i++ {
		if nextLine.MatchString(lines[i]) {
			next = i
			break
		}
	}
	// The lines between an option and the next one, indented past its
	// label, describe it.
	for i := range opts {
		end := footer
		if i > 0 {
			end = lineOf[i-1]
		}
		if next > lineOf[i] && next < end {
			end = next
		}
		opts[i].Description = description(lines, lineOf[i], end)
	}
	for i := len(opts) - 1; i >= 0; i-- {
		if next >= 0 && lineOf[i] > next {
			d.Options = append(d.Options, Option{Label: "Next", Next: true, Selected: nextLine.FindStringSubmatch(lines[next])[1] != ""})
			next = -1
		}
		d.Options = append(d.Options, opts[i])
	}
	if next >= 0 {
		d.Options = append(d.Options, Option{Label: "Next", Next: true, Selected: nextLine.FindStringSubmatch(lines[next])[1] != ""})
	}
	for i := range d.Options {
		d.Options[i].Index = i
		if d.Options[i].Checked != nil || d.Options[i].Next {
			d.Multi = true
		}
	}
	return first
}

// description joins the lines after an option's line, up to end, that are
// indented past the option's number.
func description(lines []string, at, end int) string {
	indent := indentOf(strings.Replace(lines[at], "❯", " ", 1))
	var parts []string
	for i := at + 1; i < end; i++ {
		l := lines[i]
		if isRule(l) || indentOf(l) <= indent+2 {
			break
		}
		parts = append(parts, strings.TrimSpace(l))
	}
	return strings.Join(parts, " ")
}

// markedOptions reads an unnumbered list: the line holding the selection
// marker and its neighbours whose text starts in the same column.
func markedOptions(lines []string, top, footer int, d *Dialog) int {
	sel := -1
	for i := footer - 1; i >= top; i-- {
		if strings.HasPrefix(strings.TrimLeftFunc(lines[i], unicode.IsSpace), "❯") {
			sel = i
			break
		}
	}
	if sel < 0 {
		return -1
	}
	col := textColumn(lines[sel])
	start, end := sel, sel
	for start-1 >= top && isSibling(lines[start-1], col) {
		start--
	}
	for end+1 < footer && isSibling(lines[end+1], col) {
		end++
	}
	for i := start; i <= end; i++ {
		label := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[i]), "❯"))
		d.Options = append(d.Options, option(i-start, label, i == sel))
	}
	return start
}

func option(index int, label string, selected bool) Option {
	label = strings.TrimSpace(label)
	o := Option{Index: index, Label: label, Selected: selected}
	if m := checkbox.FindStringSubmatch(label); m != nil {
		ticked := m[1] != " "
		o.Label, o.Checked = m[2], &ticked
	}
	o.Free = freeLabel.MatchString(o.Label)
	return o
}

// textColumn is where the label starts on a line with the marker.
func textColumn(line string) int {
	r := []rune(line)
	for i, c := range r {
		if c == '❯' {
			j := i + 1
			for j < len(r) && unicode.IsSpace(r[j]) {
				j++
			}
			return j
		}
	}
	return 0
}

func isSibling(line string, col int) bool {
	r := []rune(line)
	if len(r) <= col || isRule(line) {
		return false
	}
	for _, c := range r[:col] {
		if !unicode.IsSpace(c) {
			return false
		}
	}
	return !unicode.IsSpace(r[col])
}

// maxQuestionLines bounds how much text above the options is taken as the
// question.
const maxQuestionLines = 12

// question is the block of text just above the options: the nearest line
// that is not a separator, and the lines above it at the same indentation,
// up to a separator, a tab bar, a box edge, the input prompt or a change of
// indentation. The terminal wraps long questions over several lines; those
// are joined with spaces, while lines that ended early keep their breaks
// and blank lines become paragraph breaks.
func question(lines []string, gaps []bool, top, first, width int) string {
	last := -1
	for i := first - 1; i >= top && i >= first-4; i-- {
		if !isRule(lines[i]) {
			last = i
			break
		}
	}
	if last < 0 {
		return ""
	}
	indent := indentOf(unbar(lines[last]))
	start := last
	for i := last - 1; i >= top && last-i < maxQuestionLines; i-- {
		l := unbar(lines[i])
		if indentOf(l) != indent || !questionLine(l) {
			break
		}
		start = i
	}
	var b strings.Builder
	for i := start; i <= last; i++ {
		text := strings.TrimSpace(unbar(lines[i]))
		if i > start {
			prev := []rune(strings.TrimRight(lines[i-1], " "))
			word, _, _ := strings.Cut(text, " ")
			switch {
			case gaps[i]:
				b.WriteString("\n\n")
			case len(prev)+1+len([]rune(word)) > width:
				b.WriteString(" ")
			default:
				b.WriteString("\n")
			}
		}
		b.WriteString(text)
	}
	return b.String()
}

// steps reads the tab bar above a prompt with several questions, such as
// "←  ☒ Checks  ☐ Timing  ✔ Submit  →". A bar with one tab is a single
// question and gives no steps.
func steps(lines []string, top, first int) []Step {
	for i := first - 1; i >= top; i-- {
		t := strings.TrimSpace(lines[i])
		t = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(t, "←"), "→"))
		var out []Step
		for _, item := range gap.Split(t, -1) {
			m := stepItem.FindStringSubmatch(strings.TrimSpace(item))
			if m == nil {
				out = nil
				break
			}
			out = append(out, Step{Label: m[2], Done: m[1] == "☒", Submit: m[1] == "✔"})
		}
		if len(out) == 0 {
			continue
		}
		if len(out) < 2 {
			return nil
		}
		return out
	}
	return nil
}

// isRule reports a horizontal rule: dashes alone, or Claude Code's rule
// with the session's title drawn into it ("──── s3 Fidelity ─").
func isRule(l string) bool {
	return separator.MatchString(l) || titledRule(l)
}

// titledRule reports a line of at least 20 box-drawing dashes that starts
// with them and carries a short title.
func titledRule(l string) bool {
	t := strings.TrimSpace(l)
	if !strings.HasPrefix(t, "──") {
		return false
	}
	dashes := strings.Count(t, "─")
	rest := len([]rune(t)) - dashes
	return dashes >= 20 && rest <= 80
}

// unbar replaces the bar Claude Code draws down the left of a question
// (│ or ▎) with a space, keeping the text where it was.
func unbar(l string) string {
	t := strings.TrimLeft(l, " ")
	for _, bar := range []string{"│", "▎", "┃"} {
		if rest, ok := strings.CutPrefix(t, bar); ok && !strings.Contains(rest, bar) {
			return l[:len(l)-len(t)] + " " + rest
		}
	}
	return l
}

// questionLine reports whether a line can be part of a question: not a
// separator, tab bar, box edge or the input prompt.
func questionLine(l string) bool {
	t := strings.TrimSpace(l)
	if isRule(l) || strings.HasPrefix(t, "❯") || strings.HasPrefix(t, "←") {
		return false
	}
	return !strings.ContainsAny(t, "☐☒✔│╭╰┃")
}

func indentOf(l string) int {
	return len(l) - len(strings.TrimLeft(l, " "))
}

// screenWidth is the terminal width, from the widest line; separators span
// it.
func screenWidth(lines []string) int {
	w := 0
	for _, l := range lines {
		w = max(w, len([]rune(l)))
	}
	return w
}

// nonBlank returns the screen's non-blank lines, and for each whether blank
// lines came before it.
func nonBlank(screen string) ([]string, []bool) {
	var out []string
	var gaps []bool
	gap := false
	for _, l := range strings.Split(screen, "\n") {
		if strings.TrimSpace(l) == "" {
			gap = len(out) > 0
			continue
		}
		out = append(out, strings.TrimRight(l, " \t"))
		gaps = append(gaps, gap)
		gap = false
	}
	return out, gaps
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

// Moves returns the key presses that select option index and confirm it,
// starting from the current selection.
func (d *Dialog) Moves(index int) []string {
	cur := 0
	for _, o := range d.Options {
		if o.Selected {
			cur = o.Index
		}
	}
	var keys []string
	for ; cur < index; cur++ {
		keys = append(keys, "Down")
	}
	for ; cur > index; cur-- {
		keys = append(keys, "Up")
	}
	return append(keys, "Enter")
}
