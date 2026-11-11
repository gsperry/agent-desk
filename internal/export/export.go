// Package export writes a transcript as a Markdown document: prompts and
// replies in full, tool calls as one line each, failed tool output in short.
// Thinking, bookkeeping and raw records are left out.
package export

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gsperry/agent-desk/internal/transcript"
)

// Header describes the conversation at the top of the document.
type Header struct {
	Title     string
	ID        string
	Directory string
	Branch    string
}

// errorLines is how much of a failed tool's output is kept.
const errorLines = 8

// Markdown renders items as a document.
func Markdown(h Header, items []transcript.Item) string {
	var b strings.Builder
	title := h.Title
	if title == "" {
		title = "Conversation " + h.ID
	}
	fmt.Fprintf(&b, "# %s\n\n", oneLine(title))
	if h.Directory != "" {
		fmt.Fprintf(&b, "- Directory: %s\n", code(h.Directory))
	}
	if h.Branch != "" {
		fmt.Fprintf(&b, "- Branch: %s\n", code(h.Branch))
	}
	if h.ID != "" {
		fmt.Fprintf(&b, "- Conversation: %s\n", code(h.ID))
	}
	b.WriteString("\n")

	speaker := ""
	inTools := false
	heading := func(who, at string) {
		if inTools {
			b.WriteString("\n")
			inTools = false
		}
		if who == speaker {
			return
		}
		speaker = who
		if t := clock(at); t != "" {
			fmt.Fprintf(&b, "## %s · %s\n\n", who, t)
		} else {
			fmt.Fprintf(&b, "## %s\n\n", who)
		}
	}
	for _, it := range items {
		if it.Sidechain {
			continue
		}
		switch it.Kind {
		case transcript.KindUser, transcript.KindPeer:
			if strings.TrimSpace(it.Text) == "" {
				continue
			}
			who := "You"
			if it.Kind == transcript.KindPeer {
				who = "Another agent"
			}
			speaker = "" // every prompt starts a section
			heading(who, it.Time)
			b.WriteString(strings.TrimSpace(it.Text) + "\n\n")
		case transcript.KindText:
			if strings.TrimSpace(it.Text) == "" {
				continue
			}
			heading("Claude", it.Time)
			b.WriteString(strings.TrimSpace(it.Text) + "\n\n")
		case transcript.KindToolUse:
			if it.Tool == nil {
				continue
			}
			if speaker != "Claude" {
				heading("Claude", it.Time)
			}
			fmt.Fprintf(&b, "- **%s**%s\n", it.Tool.Name, toolDetail(it.Tool.Input))
			inTools = true
		case transcript.KindToolResult:
			if it.Result == nil || !it.Result.IsError {
				continue
			}
			fmt.Fprintf(&b, "  - failed:\n\n%s\n", indent(fence(firstLines(it.Result.Content, errorLines)), "    "))
			inTools = true
		case transcript.KindCompactSummary, transcript.KindCompact:
			heading("Context compacted", it.Time)
			speaker = ""
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// toolDetail picks the most telling input of a tool call.
func toolDetail(raw json.RawMessage) string {
	var in map[string]any
	if json.Unmarshal(raw, &in) != nil {
		return ""
	}
	for _, k := range []string{"command", "file_path", "notebook_path", "pattern", "url", "query", "description", "prompt"} {
		if v, ok := in[k].(string); ok && strings.TrimSpace(v) != "" {
			return ": " + code(clip(oneLine(v), 200))
		}
	}
	return ""
}

var fenceRun = regexp.MustCompile("`{3,}")

// fence wraps text in a code fence longer than any backtick run inside it.
func fence(s string) string {
	ticks := "```"
	for _, m := range fenceRun.FindAllString(s, -1) {
		if len(m) >= len(ticks) {
			ticks = strings.Repeat("`", len(m)+1)
		}
	}
	return ticks + "\n" + s + "\n" + ticks
}

// code wraps text in inline code with enough backticks to hold it.
func code(s string) string {
	ticks := "`"
	for strings.Contains(s, ticks) {
		ticks += "`"
	}
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") {
		return ticks + " " + s + " " + ticks
	}
	return ticks + s + ticks
}

func indent(s, prefix string) string {
	return prefix + strings.ReplaceAll(s, "\n", "\n"+prefix)
}

func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = append(lines[:n], "…")
	}
	return strings.Join(lines, "\n")
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// clock formats an RFC 3339 time as a UTC date and minute.
func clock(at string) string {
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9]+`)

// Filename turns a title into a safe download name.
func Filename(title, id string) string {
	base := strings.Trim(unsafeName.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if len(base) > 60 {
		base = strings.TrimRight(base[:60], "-")
	}
	if base == "" {
		base = "conversation-" + unsafeName.ReplaceAllString(id[:min(len(id), 8)], "")
	}
	return base + ".md"
}
