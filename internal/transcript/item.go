// Package transcript parses Claude Code session JSONL into view items.
//
// The format is undocumented and changes between releases, so the parser is
// tolerant: a record or content block it does not recognize, and any line
// that is not valid JSON, becomes a KindRaw item carrying the original text.
// Nothing returns an error and nothing is dropped without being counted.
package transcript

import "encoding/json"

type Kind string

const (
	// KindUser is a prompt the human wrote.
	KindUser Kind = "user"
	// KindPeer is a message from another agent session.
	KindPeer Kind = "peer"
	// KindNotice is text the harness injected into a user turn: slash
	// commands and their output, task notifications, reminders, interrupts.
	KindNotice         Kind = "notice"
	KindCompactSummary Kind = "compact_summary"
	KindText           Kind = "text"
	KindThinking       Kind = "thinking"
	KindToolUse        Kind = "tool_use"
	KindToolResult     Kind = "tool_result"
	KindImage          Kind = "image"
	KindSystem         Kind = "system"
	KindCompact        Kind = "compact_boundary"
	KindAttachment     Kind = "attachment"
	// KindMeta carries session metadata (titles, modes, PR links) that the
	// header reads rather than the transcript showing inline.
	KindMeta Kind = "meta"
	KindRaw  Kind = "raw"
)

// Item is one renderable unit. Fields other than Seq and Kind are set per kind.
type Item struct {
	Seq       int    `json:"seq"`
	Kind      Kind   `json:"kind"`
	UUID      string `json:"uuid,omitempty"`
	Time      string `json:"time,omitempty"`
	MessageID string `json:"messageId,omitempty"`
	Sidechain bool   `json:"sidechain,omitempty"`
	// Label names the variant: notice type, system subtype, attachment type,
	// meta record type, or the reason an item is raw.
	Label string `json:"label,omitempty"`
	Text  string `json:"text,omitempty"`
	// Preamble is agent-manager's launch directive, split from the prompt.
	Preamble string         `json:"preamble,omitempty"`
	Tool     *ToolUse       `json:"tool,omitempty"`
	Result   *ToolResult    `json:"result,omitempty"`
	Meta     map[string]any `json:"meta,omitempty"`
	// Raw is the original record, capped, for KindRaw.
	Raw string `json:"raw,omitempty"`
}

type ToolUse struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input,omitempty"`
}

type ToolResult struct {
	ToolUseID string `json:"toolUseId"`
	Content   string `json:"content"`
	IsError   bool   `json:"isError,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	// Detail is the record's toolUseResult with heavy fields removed:
	// structuredPatch for edits, stdout and stderr for Bash, answers for
	// AskUserQuestion.
	Detail json.RawMessage `json:"detail,omitempty"`
}

// Stats counts what a parse saw, so hidden records are accounted for.
type Stats struct {
	Lines      int            `json:"lines"`
	Items      int            `json:"items"`
	Raw        int            `json:"raw"`
	Duplicates int            `json:"duplicates"`
	Skipped    map[string]int `json:"skipped,omitempty"`
}
