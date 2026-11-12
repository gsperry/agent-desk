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
}

type Option struct {
	Index    int    `json:"index"`
	Label    string `json:"label"`
	Selected bool   `json:"selected"`
	// Free options are a text field in the dialog ("Type something."):
	// they are answered with text, not a key press.
	Free bool `json:"free,omitempty"`
}

var (
	footerPattern = regexp.MustCompile(`Enter to (?:confirm|select)|Esc to cancel`)
	numbered      = regexp.MustCompile(`^(\s*)(❯)?\s*(\d{1,2})\.\s+(.+?)\s*$`)
	separator     = regexp.MustCompile(`^\s*[─╌━-]{10,}\s*$`)
	// freeLabel is an option that is a text field. "Chat about this" is not
	// one: choosing it declines the question and returns to the prompt.
	freeLabel = regexp.MustCompile(`^Type something`)
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
		opts = append(opts, option(n-1, m[4], m[2] != ""))
		want, first = n, i
		if n == 1 {
			break
		}
	}
	if want != 1 {
		return -1
	}
	for i := len(opts) - 1; i >= 0; i-- {
		d.Options = append(d.Options, opts[i])
	}
	return first
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
	return Option{Index: index, Label: label, Selected: selected, Free: freeLabel.MatchString(label)}
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
	if len(r) <= col || separator.MatchString(line) {
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
		if !separator.MatchString(lines[i]) {
			last = i
			break
		}
	}
	if last < 0 {
		return ""
	}
	indent := indentOf(lines[last])
	start := last
	for i := last - 1; i >= top && last-i < maxQuestionLines; i-- {
		if indentOf(lines[i]) != indent || !questionLine(lines[i]) {
			break
		}
		start = i
	}
	var b strings.Builder
	for i := start; i <= last; i++ {
		text := strings.TrimSpace(lines[i])
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

// questionLine reports whether a line can be part of a question: not a
// separator, tab bar, box edge or the input prompt.
func questionLine(l string) bool {
	t := strings.TrimSpace(l)
	if separator.MatchString(l) || strings.HasPrefix(t, "❯") || strings.HasPrefix(t, "←") {
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
