package transcript

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const (
	maxContent = 64 << 10
	maxRaw     = 16 << 10

	labelTaskNotification = "task-notification"
)

// skipped records are bookkeeping with no reading value. They are counted in
// Stats rather than emitted; request payload copies alone can be most of a file.
var skipped = map[string]bool{
	"api-request": true, "api-request-blob": true, "api-request-shape": true,
	"queue-operation": true, "file-history-snapshot": true, "file-history-delta": true,
	"atis-latch": true, "tools-used": true, "cost-state": true, "last-prompt": true,
}

// metaFields maps metadata record types to the fields the header reads.
var metaFields = map[string][]string{
	"ai-title":        {"aiTitle"},
	"custom-title":    {"customTitle"},
	"agent-name":      {"agentName"},
	"mode":            {"mode"},
	"permission-mode": {"permissionMode"},
	"pr-link":         {"prNumber", "prUrl", "prRepository"},
	"continued-in":    {"continuedInSessionId"},
	"agent-setting":   {"agentSetting"},
}

// Parser turns lines into items. It keeps state across lines (sequence
// numbers, duplicate detection), so use one Parser per transcript file.
type Parser struct {
	stats Stats
	seq   int
	seen  map[string]bool
	// seenContent catches assistant records Claude Code rewrites with a new
	// uuid but the same message id and content, as after a version upgrade.
	seenContent map[string]bool
}

func NewParser() *Parser {
	return &Parser{seen: map[string]bool{}, seenContent: map[string]bool{}, stats: Stats{Skipped: map[string]int{}}}
}

func (p *Parser) Stats() Stats { return p.stats }

type record struct {
	Type             string          `json:"type"`
	Subtype          string          `json:"subtype"`
	UUID             string          `json:"uuid"`
	Timestamp        string          `json:"timestamp"`
	IsSidechain      bool            `json:"isSidechain"`
	IsMeta           bool            `json:"isMeta"`
	IsCompactSummary bool            `json:"isCompactSummary"`
	PromptSource     string          `json:"promptSource"`
	Origin           json.RawMessage `json:"origin"`
	Message          json.RawMessage `json:"message"`
	ToolUseResult    json.RawMessage `json:"toolUseResult"`
	Content          json.RawMessage `json:"content"`
	Attachment       json.RawMessage `json:"attachment"`
	Level            string          `json:"level"`
	Error            json.RawMessage `json:"error"`
	DurationMs       *float64        `json:"durationMs"`
	ThinkingMs       *float64        `json:"thinkingDurationMs"`
}

type message struct {
	ID      string          `json:"id"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
}

type block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
	Source    struct {
		MediaType string `json:"media_type"`
	} `json:"source"`
}

// Line parses one JSONL line into zero or more items.
func (p *Parser) Line(line []byte) []Item {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return nil
	}
	p.stats.Lines++
	var r record
	if err := json.Unmarshal(line, &r); err != nil {
		return p.emit(rawItem(line, "malformed: "+shortErr(err)))
	}
	if r.Type == "" {
		return p.emit(rawItem(line, "record without a type"))
	}
	if skipped[r.Type] {
		p.stats.Skipped[r.Type]++
		return nil
	}
	if r.UUID != "" {
		if p.seen[r.UUID] {
			p.stats.Duplicates++
			return nil
		}
		p.seen[r.UUID] = true
	}
	base := Item{UUID: r.UUID, Time: r.Timestamp, Sidechain: r.IsSidechain}
	var items []Item
	switch {
	case r.Type == "user":
		items = p.user(base, r, line)
	case r.Type == "assistant":
		items = p.assistant(base, r, line)
	case r.Type == "system":
		items = []Item{system(base, r)}
	case r.Type == "attachment":
		items = []Item{attachment(base, r, line)}
	case metaFields[r.Type] != nil:
		items = []Item{meta(base, r.Type, line)}
	default:
		items = []Item{withBase(base, rawItem(line, "unknown record type "+r.Type))}
	}
	return p.emit(items...)
}

func (p *Parser) emit(items ...Item) []Item {
	for i := range items {
		items[i].Seq = p.seq
		p.seq++
		p.stats.Items++
		if items[i].Kind == KindRaw {
			p.stats.Raw++
		}
	}
	return items
}

func withBase(base Item, it Item) Item {
	it.UUID, it.Time, it.Sidechain = base.UUID, base.Time, base.Sidechain
	return it
}

func rawItem(line []byte, reason string) Item {
	raw := string(line)
	if len(raw) > maxRaw {
		raw = raw[:maxRaw] + "…"
	}
	return Item{Kind: KindRaw, Label: reason, Raw: raw}
}

func shortErr(err error) string {
	s := err.Error()
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}

func decodeMessage(raw json.RawMessage) (message, bool) {
	var m message
	if len(raw) == 0 || json.Unmarshal(raw, &m) != nil {
		return m, false
	}
	return m, true
}

// contentBlocks reads message content, which is either a string or a list
// of blocks.
func contentBlocks(raw json.RawMessage) (text string, blocks []json.RawMessage, ok bool) {
	if json.Unmarshal(raw, &text) == nil {
		return text, nil, true
	}
	if json.Unmarshal(raw, &blocks) == nil {
		return "", blocks, true
	}
	return "", nil, false
}

func (p *Parser) user(base Item, r record, line []byte) []Item {
	m, ok := decodeMessage(r.Message)
	if !ok {
		return []Item{withBase(base, rawItem(line, "user record without a message"))}
	}
	text, blocks, ok := contentBlocks(m.Content)
	if !ok {
		return []Item{withBase(base, rawItem(line, "user content is neither text nor blocks"))}
	}
	if blocks == nil {
		return p.userText(base, r, text)
	}
	var items []Item
	for _, braw := range blocks {
		var b block
		if json.Unmarshal(braw, &b) != nil {
			items = append(items, withBase(base, rawItem(braw, "malformed content block")))
			continue
		}
		switch b.Type {
		case "text":
			items = append(items, p.userText(base, r, b.Text)...)
		case "tool_result":
			it := base
			it.Kind = KindToolResult
			it.Result = toolResult(b, r.ToolUseResult)
			items = append(items, it)
		case "image":
			it := base
			it.Kind = KindImage
			it.Label = b.Source.MediaType
			items = append(items, it)
		default:
			items = append(items, withBase(base, rawItem(braw, "unknown user block "+b.Type)))
		}
	}
	return items
}

var (
	preamblePattern = regexp.MustCompile(`(?s)\A(.*?agent-manager rename "<name>".*?Then do the task:)\s*(.*)\z`)
	deferredPattern = regexp.MustCompile(`(?s)\A.*agent-manager rename "<name>".*Then continue\.\s*\z`)
	commandName     = regexp.MustCompile(`(?s)<command-name>(.*?)</command-name>`)
	commandArgs     = regexp.MustCompile(`(?s)<command-args>(.*?)</command-args>`)
	tagged          = regexp.MustCompile(`(?s)\A\s*<([a-z][a-z0-9_-]*)>(.*?)(?:</[a-z][a-z0-9_-]*>)?\s*\z`)
	reminder        = regexp.MustCompile(`(?s)<system-reminder>(.*?)</system-reminder>`)
)

// userText classifies one piece of user text. Reminders embedded in a
// human prompt are split out so the prompt reads as written.
func (p *Parser) userText(base Item, r record, text string) []Item {
	it := p.classifyUserText(base, r, text)
	if it.Kind != KindUser || !strings.Contains(text, "<system-reminder>") {
		return []Item{it}
	}
	var items []Item
	for _, m := range reminder.FindAllStringSubmatch(it.Text, -1) {
		n := base
		n.Kind, n.Label, n.Text = KindNotice, "system-reminder", strings.TrimSpace(m[1])
		items = append(items, n)
	}
	it.Text = strings.TrimSpace(reminder.ReplaceAllString(it.Text, ""))
	if it.Text == "" && it.Preamble == "" {
		return items
	}
	return append([]Item{it}, items...)
}

func originKind(raw json.RawMessage) string {
	var o struct {
		Kind string `json:"kind"`
	}
	if len(raw) > 0 && json.Unmarshal(raw, &o) == nil {
		return o.Kind
	}
	return ""
}

func (p *Parser) classifyUserText(base Item, r record, text string) Item {
	it := base
	trimmed := strings.TrimSpace(text)
	notice := func(label, body string) Item {
		it.Kind, it.Label, it.Text = KindNotice, label, StripANSI(strings.TrimSpace(body))
		return it
	}
	origin := originKind(r.Origin)
	switch {
	case r.IsCompactSummary:
		it.Kind, it.Text = KindCompactSummary, text
		return it
	case origin == labelTaskNotification || strings.HasPrefix(trimmed, "<task-notification>"):
		return notice(labelTaskNotification, trimmed)
	case origin == "peer":
		it.Kind, it.Text = KindPeer, text
		return it
	case strings.HasPrefix(trimmed, "<command-name>") || strings.HasPrefix(trimmed, "<command-message>"):
		cmd := ""
		if m := commandName.FindStringSubmatch(trimmed); m != nil {
			cmd = strings.TrimSpace(m[1])
		}
		if m := commandArgs.FindStringSubmatch(trimmed); m != nil && strings.TrimSpace(m[1]) != "" {
			cmd += " " + strings.TrimSpace(m[1])
		}
		return notice("command", cmd)
	case strings.HasPrefix(trimmed, "[Request interrupted"):
		return notice("interrupted", trimmed)
	case deferredPattern.MatchString(trimmed):
		return notice("agent-manager", trimmed)
	}
	if m := tagged.FindStringSubmatch(trimmed); m != nil {
		switch m[1] {
		case "local-command-stdout", "local-command-stderr":
			return notice("command-output", m[2])
		case "bash-input":
			return notice("bash-input", m[2])
		case "bash-stdout", "bash-stderr":
			return notice("bash-output", m[2])
		case "local-command-caveat", "system-reminder", "ide_opened_file", "ide_selection", "user-prompt-submit-hook":
			return notice(m[1], m[2])
		}
	}
	if r.IsMeta {
		return notice("injected", trimmed)
	}
	it.Kind, it.Text = KindUser, text
	if m := preamblePattern.FindStringSubmatch(text); m != nil {
		it.Preamble, it.Text = m[1], m[2]
	}
	if r.PromptSource != "" {
		it.Meta = map[string]any{"source": r.PromptSource}
	}
	return it
}

func (p *Parser) assistant(base Item, r record, line []byte) []Item {
	m, ok := decodeMessage(r.Message)
	if !ok {
		return []Item{withBase(base, rawItem(line, "assistant record without a message"))}
	}
	base.MessageID = m.ID
	if m.ID != "" {
		key := m.ID + "\x00" + string(m.Content)
		if p.seenContent[key] {
			p.stats.Duplicates++
			return nil
		}
		p.seenContent[key] = true
	}
	text, blocks, ok := contentBlocks(m.Content)
	if !ok {
		return []Item{withBase(base, rawItem(line, "assistant content is neither text nor blocks"))}
	}
	if blocks == nil {
		it := base
		it.Kind, it.Text = KindText, text
		return []Item{it}
	}
	var items []Item
	for _, braw := range blocks {
		var b block
		if json.Unmarshal(braw, &b) != nil {
			items = append(items, withBase(base, rawItem(braw, "malformed content block")))
			continue
		}
		it := base
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) == "(no content)" {
				// Placeholder some versions write while a reply streams.
				continue
			}
			it.Kind, it.Text = KindText, b.Text
		case "thinking", "redacted_thinking":
			it.Kind, it.Text = KindThinking, b.Thinking
			if r.ThinkingMs != nil {
				it.Meta = map[string]any{"durationMs": *r.ThinkingMs}
			}
		case "tool_use", "server_tool_use":
			it.Kind = KindToolUse
			it.Tool = &ToolUse{ID: b.ID, Name: b.Name, Input: capJSON(b.Input)}
		default:
			it = withBase(base, rawItem(braw, "unknown assistant block "+b.Type))
			it.MessageID = m.ID
		}
		items = append(items, it)
	}
	return items
}

func toolResult(b block, detail json.RawMessage) *ToolResult {
	res := &ToolResult{ToolUseID: b.ToolUseID, IsError: b.IsError}
	text, blocks, _ := contentBlocks(b.Content)
	if blocks != nil {
		var parts []string
		for _, braw := range blocks {
			var inner block
			if json.Unmarshal(braw, &inner) != nil {
				continue
			}
			switch inner.Type {
			case "text":
				parts = append(parts, inner.Text)
			case "image":
				parts = append(parts, "[image]")
			default:
				parts = append(parts, fmt.Sprintf("[%s]", inner.Type))
			}
		}
		text = strings.Join(parts, "\n")
	}
	text = StripANSI(text)
	if len(text) > maxContent {
		text, res.Truncated = text[:maxContent], true
	}
	res.Content = text
	res.Detail = slimDetail(detail)
	return res
}

// slimDetail drops whole-file copies from toolUseResult, strips ANSI codes
// and caps long strings, keeping the structured fields the viewer renders.
func slimDetail(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		delete(m, "originalFile")
	}
	out, err := json.Marshal(capStrings(v))
	if err != nil {
		return nil
	}
	return out
}

func capStrings(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, c := range t {
			t[k] = capStrings(c)
		}
	case []any:
		for i, c := range t {
			t[i] = capStrings(c)
		}
	case string:
		s := StripANSI(t)
		if len(s) > maxContent {
			return s[:maxContent] + "\n… (truncated)"
		}
		return s
	}
	return v
}

func capJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) <= maxContent {
		return raw
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return nil
	}
	out, _ := json.Marshal(capStrings(v))
	return out
}

func system(base Item, r record) Item {
	it := base
	it.Kind, it.Label = KindSystem, r.Subtype
	if r.Subtype == "compact_boundary" {
		it.Kind = KindCompact
	}
	var text string
	if json.Unmarshal(r.Content, &text) == nil {
		it.Text = StripANSI(text)
	}
	m := map[string]any{}
	if r.DurationMs != nil {
		m["durationMs"] = *r.DurationMs
	}
	if r.Level != "" {
		m["level"] = r.Level
	}
	if len(r.Error) > 0 && string(r.Error) != "null" {
		var e any
		if json.Unmarshal(r.Error, &e) == nil {
			m["error"] = e
		}
	}
	if len(m) > 0 {
		it.Meta = m
	}
	return it
}

func attachment(base Item, r record, line []byte) Item {
	var a map[string]any
	if json.Unmarshal(r.Attachment, &a) != nil {
		return withBase(base, rawItem(line, "attachment without a body"))
	}
	it := base
	it.Kind = KindAttachment
	it.Label, _ = a["type"].(string)
	if it.Label != "queued_command" {
		return it
	}
	// A prompt submitted while the agent was busy is recorded only as this
	// attachment, never as a user record, so it renders as the prompt itself.
	text := promptText(a["prompt"])
	origin, _ := a["origin"].(map[string]any)
	switch kind, _ := origin["kind"].(string); kind {
	case "human":
		it.Kind, it.Label, it.Text = KindUser, "", text
		it.Meta = map[string]any{"queued": true}
	case labelTaskNotification:
		it.Kind, it.Label, it.Text = KindNotice, labelTaskNotification, strings.TrimSpace(text)
	default:
		it.Text = text
	}
	return it
}

func promptText(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		var parts []string
		for _, b := range t {
			if m, ok := b.(map[string]any); ok {
				if s, ok := m["text"].(string); ok {
					parts = append(parts, s)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func meta(base Item, typ string, line []byte) Item {
	var all map[string]any
	_ = json.Unmarshal(line, &all)
	it := base
	it.Kind, it.Label = KindMeta, typ
	it.Meta = map[string]any{}
	for _, f := range metaFields[typ] {
		if v, ok := all[f]; ok {
			it.Meta[f] = v
		}
	}
	return it
}

var ansi = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[@-Z\\-_]`)

// StripANSI removes terminal escape sequences.
func StripANSI(s string) string {
	if !strings.Contains(s, "\x1b") {
		return s
	}
	return ansi.ReplaceAllString(s, "")
}
