package pane

import (
	"regexp"
	"strings"
)

// sgr is one color or style escape sequence.
var sgr = regexp.MustCompile("\x1b\\[([0-9;]*)m")

// currentStep finds which question of a multi-question prompt is showing.
// The plain screen cannot tell: Claude Code marks the current tab only by
// giving it a background color. styled is the screen with its escape
// sequences (agent-manager read --styled). It returns the step's index,
// or -1 when the tab bar or its highlight cannot be found.
func currentStep(styled string, steps []Step) int {
	if len(steps) < 2 {
		return -1
	}
	for _, line := range strings.Split(styled, "\n") {
		plain := sgr.ReplaceAllString(line, "")
		if !strings.Contains(plain, "→") || !hasAllSteps(plain, steps) {
			continue
		}
		label := highlighted(line)
		if label == "" {
			return -1
		}
		for i, s := range steps {
			if s.Label == label {
				return i
			}
		}
		return -1
	}
	return -1
}

func hasAllSteps(plain string, steps []Step) bool {
	for _, s := range steps {
		if !strings.Contains(plain, s.Label) {
			return false
		}
	}
	return true
}

// highlighted returns the step label in the part of line drawn on a
// background color, without its ☐, ☒ or ✔ mark.
func highlighted(line string) string {
	var b strings.Builder
	on := false
	for _, part := range splitSGR(line) {
		if part.codes != nil {
			for _, c := range part.codes {
				switch {
				case c == "48" || strings.HasPrefix(c, "48:"):
					on = true
				case c == "49" || c == "0" || c == "":
					if on && b.Len() > 0 {
						return stepLabel(b.String())
					}
					on = false
				}
			}
			continue
		}
		if on {
			b.WriteString(part.text)
		}
	}
	if on {
		return stepLabel(b.String())
	}
	return ""
}

func stepLabel(s string) string {
	s = strings.TrimSpace(s)
	for _, mark := range []string{"☐", "☒", "✔"} {
		s = strings.TrimSpace(strings.TrimPrefix(s, mark))
	}
	return s
}

type sgrPart struct {
	text string
	// codes are the sequence's parameters split on ";"; nil for text.
	codes []string
}

// splitSGR cuts a line into text and escape sequences.
func splitSGR(line string) []sgrPart {
	var out []sgrPart
	last := 0
	for _, m := range sgr.FindAllStringSubmatchIndex(line, -1) {
		if m[0] > last {
			out = append(out, sgrPart{text: line[last:m[0]]})
		}
		out = append(out, sgrPart{codes: sgrCodes(line[m[2]:m[3]])})
		last = m[1]
	}
	if last < len(line) {
		out = append(out, sgrPart{text: line[last:]})
	}
	return out
}

// sgrCodes splits parameters, folding the color values after 38 and 48
// into them, so "48;5;153" is one code and its 153 is not read as a
// separate code.
func sgrCodes(params string) []string {
	f := strings.Split(params, ";")
	var out []string
	for i := 0; i < len(f); i++ {
		c := f[i]
		if (c == "38" || c == "48") && i+1 < len(f) {
			n := 2
			if f[i+1] == "2" {
				n = 4
			}
			end := min(i+1+n, len(f))
			c = strings.Join(f[i:end], ":")
			i = end - 1
		}
		out = append(out, c)
	}
	return out
}
