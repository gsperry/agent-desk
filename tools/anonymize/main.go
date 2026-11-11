// Command anonymize turns a real Claude Code transcript into a test sample.
//
// It keeps every record's structure, keys, ids, timestamps, versions, tool
// names and the markers the parser classifies by, and replaces every other
// letter and digit with filler of the same class, so lengths, whitespace,
// punctuation and markdown survive. JSON numbers are kept. Large request
// payload records are dropped after the first few, and an optional record
// cap keeps samples small.
//
//	go run ./tools/anonymize [-max 600] in.jsonl > out.jsonl
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"log"
	"os"
	"regexp"
	"strings"
)

// keepKeys hold structural values the parser and the UI key on.
var keepKeys = map[string]bool{
	"type": true, "subtype": true, "role": true, "id": true, "uuid": true,
	"parentUuid": true, "logicalParentUuid": true, "tool_use_id": true,
	"sessionId": true, "session_id": true, "timestamp": true, "version": true,
	"model": true, "requestedModel": true, "stop_reason": true, "stopReason": true,
	"operation": true, "promptSource": true, "kind": true, "permissionMode": true,
	"mode": true, "level": true, "media_type": true, "requestId": true,
	"messageId": true, "leafUuid": true, "agentId": true, "toolUseID": true,
	"sourceToolUseID": true, "sourceToolAssistantUUID": true, "promptId": true,
	"userType": true, "entrypoint": true, "effort": true, "thinkingDisplay": true,
	"sessionKind": true, "continuedInSessionId": true, "snapshotMessageId": true,
	"interruptedMessageId": true, "toolDenialKind": true, "cronKind": true,
	"taskKind": true, "querySource": true, "turnOrigin": true, "origin": true,
}

// preserve are spans of text kept verbatim inside replaced strings.
var preserve = regexp.MustCompile(strings.Join([]string{
	`(?s)\A.*?agent-manager rename "<name>".*?Then (?:do the task:|continue\.)`,
	"```[A-Za-z0-9_+-]*",
	`</?[A-Za-z][A-Za-z0-9_-]*>`,
	`\[Request interrupted[^\]]*\]`,
	// Ordered list markers: CommonMark lets only a list starting at 1
	// interrupt a paragraph, so scrambling them changes the rendering.
	`(?m)^[ \t]*\d{1,3}[.)][ \t]`,
}, "|"))

const filler = "loremipsumdolorsitametconsecteturadipiscingelitseddoeiusmodtemporincididunt"

// tinyPNG is a 1x1 transparent PNG, standing in for pasted images.
const tinyPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="

func scramble(s string) string {
	h := fnv.New32a()
	h.Write([]byte(s))
	seed := int(h.Sum32() % uint32(len(filler)))
	var b strings.Builder
	last := 0
	for _, loc := range preserve.FindAllStringIndex(s, -1) {
		b.WriteString(letters(s[last:loc[0]], seed+last))
		b.WriteString(s[loc[0]:loc[1]])
		last = loc[1]
	}
	b.WriteString(letters(s[last:], seed+last))
	return b.String()
}

func letters(s string, seed int) string {
	out := []rune(s)
	for i, r := range out {
		f := rune(filler[(seed+i)%len(filler)])
		switch {
		case r >= 'a' && r <= 'z':
			out[i] = f
		case r >= 'A' && r <= 'Z':
			out[i] = f - 'a' + 'A'
		case r >= '0' && r <= '9':
			out[i] = '0' + (f-'a'+r)%10
		case r > 127 && (r < 0x2000 || r > 0x2BFF):
			// Non-ASCII letters (names, CJK) become filler; symbols such as
			// arrows and box drawing stay.
			out[i] = f
		}
	}
	return string(out)
}

func walk(v any, key string, parent map[string]any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, child := range t {
			nk := k
			// Schema keys never hold spaces or slashes; keys that do are
			// data, such as question text or file paths.
			if strings.ContainsAny(k, " /") {
				nk = scramble(k)
			}
			out[nk] = walk(child, k, t)
		}
		return out
	case []any:
		for i, child := range t {
			t[i] = walk(child, key, parent)
		}
		return t
	case string:
		switch {
		case keepKeys[key]:
			return t
		case key == "name" && parent != nil && (parent["type"] == "tool_use" || parent["input"] != nil):
			return t
		case key == "signature":
			return "sig"
		case key == "data" && parent != nil && parent["media_type"] != nil:
			return tinyPNG
		}
		return scramble(t)
	default:
		return v
	}
}

func main() {
	max := flag.Int("max", 0, "keep at most this many records (0 keeps all)")
	flag.Parse()
	if flag.NArg() != 1 {
		log.Fatal("usage: anonymize [-max n] in.jsonl > out.jsonl")
	}
	f, err := os.Open(flag.Arg(0))
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	kept, payloads := 0, 0
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			// Malformed lines are part of what the parser must survive.
			fmt.Fprintln(out, scramble(string(line)))
			kept++
			continue
		}
		if t, _ := rec["type"].(string); strings.HasPrefix(t, "api-request") {
			if payloads++; payloads > 3 {
				continue
			}
		}
		enc, err := json.Marshal(walk(rec, "", nil))
		if err != nil {
			log.Fatal(err)
		}
		out.Write(enc)
		out.WriteByte('\n')
		if kept++; *max > 0 && kept >= *max {
			break
		}
	}
	if err := sc.Err(); err != nil {
		log.Fatal(err)
	}
}
