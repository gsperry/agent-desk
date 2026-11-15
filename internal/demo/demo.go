// Package demo writes a made-up set of agent-manager sessions and Claude
// Code transcripts, for agent-desk --demo and for the website's screenshots
// and video. Nothing in it is real: the sessions, paths, prompts and system
// stats are invented. Write fills a directory that fixture mode serves.
package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Session ids and conversations the demo's live behavior and the site
// tooling refer to.
const (
	// Waiting is flaky-auth-test, waiting on a permission prompt.
	Waiting = "d0000002"
)

// ConvID is the conversation id of demo conversation n.
func ConvID(n int) string {
	return fmt.Sprintf("%08x-5e5e-4d00-8000-%012x", n, n)
}

func ts(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

type record = map[string]any

// conv builds one transcript.
type conv struct {
	id    string
	t     time.Time
	lines []record
	n     int
	model string
	cwd   string
}

func newConv(id string, start time.Time, cwd string) *conv {
	return &conv{id: id, t: start, model: "claude-opus-4-1", cwd: cwd}
}

func (c *conv) uuid() string {
	c.n++
	return fmt.Sprintf("%s-%04d", c.id[:8], c.n)
}

func (c *conv) tick(seconds float64) {
	c.t = c.t.Add(time.Duration(seconds * float64(time.Second)))
}

func usage(in, out, ctx int) record {
	return record{"input_tokens": in, "output_tokens": out, "cache_read_input_tokens": ctx - in, "cache_creation_input_tokens": 0}
}

func (c *conv) prompt(text string, gap float64) {
	c.tick(gap)
	c.lines = append(c.lines, record{
		"type": "user", "uuid": c.uuid(), "timestamp": ts(c.t), "cwd": c.cwd,
		"gitBranch": "main", "promptSource": "typed", "origin": record{"kind": "human"},
		"message": record{"role": "user", "content": text},
	})
}

func (c *conv) reply(text string, ctx, out int, gap float64) {
	c.tick(gap)
	c.lines = append(c.lines, record{
		"type": "assistant", "uuid": c.uuid(), "timestamp": ts(c.t),
		"message": record{
			"id": "msg_" + c.uuid(), "model": c.model, "role": "assistant",
			"content": []record{{"type": "text", "text": text}},
			"usage":   usage(1200, out, ctx),
		},
	})
}

type toolOpts struct {
	ctx    int
	detail record
	err    bool
	id     string
}

func (c *conv) tool(name string, input record, result string, o toolOpts) string {
	if o.ctx == 0 {
		o.ctx = 60000
	}
	c.tick(6)
	id := o.id
	if id == "" {
		id = "toolu_" + strings.ReplaceAll(c.uuid(), "-", "")
	}
	c.lines = append(c.lines, record{
		"type": "assistant", "uuid": c.uuid(), "timestamp": ts(c.t),
		"message": record{
			"id": "msg_" + c.uuid(), "model": c.model, "role": "assistant",
			"content": []record{{"type": "tool_use", "id": id, "name": name, "input": input}},
			"usage":   usage(900, 120, o.ctx),
		},
	})
	c.tick(3)
	res := record{"type": "tool_result", "tool_use_id": id, "content": result}
	if o.err {
		res["is_error"] = true
	}
	rec := record{"type": "user", "uuid": c.uuid(), "timestamp": ts(c.t), "message": record{"role": "user", "content": []record{res}}}
	if o.detail != nil {
		rec["toolUseResult"] = o.detail
	}
	c.lines = append(c.lines, rec)
	return id
}

func (c *conv) title(text string) {
	c.lines = append(c.lines, record{"type": "ai-title", "aiTitle": text, "sessionId": c.id})
}

func (c *conv) cost(usd float64) {
	c.lines = append(c.lines, record{"type": "cost-state", "startTime": 1, "totalCostUSD": usd})
}

func (c *conv) duration(ms int) {
	c.lines = append(c.lines, record{"type": "system", "uuid": c.uuid(), "subtype": "turn_duration", "durationMs": ms})
}

// write saves the transcript under claude/projects/<project> and returns
// that directory.
func (c *conv) write(dir, project string) (string, error) {
	d := filepath.Join(dir, "claude", "projects", project)
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return d, writeLines(filepath.Join(d, c.id+".jsonl"), c.lines)
}

func writeLines(path string, lines []record) error {
	var b strings.Builder
	for _, l := range lines {
		data, err := json.Marshal(l)
		if err != nil {
			return err
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

type session struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Tool           string `json:"tool"`
	Group          string `json:"group"`
	Directory      string `json:"directory"`
	Status         string `json:"status"`
	Running        bool   `json:"running"`
	Archived       bool   `json:"archived"`
	AgentSessionID string `json:"agent_session_id"`
	Self           bool   `json:"self"`
}

func newSession(i int, name, group, directory, status, convID, tool string) session {
	return session{ID: fmt.Sprintf("d000%04d", i), Name: name, Tool: tool, Group: group, Directory: directory,
		Status: status, Running: status != "dead", AgentSessionID: convID}
}

// WaitingPane is the waiting session's screen: a permission prompt for
// the edit it proposes.
const WaitingPane = `● Reproduced on run 7. The test session has a 300 ms TTL and the
  assertion sometimes runs after it expires.

● Update(auth/testdata/fixture.go)
  ⎿  -  TTL: 300 * time.Millisecond,
     +  TTL: 5 * time.Second,

────────────────────────────────────────────────────────────────────────
 Edit file
 auth/testdata/fixture.go

 Do you want to make this edit to fixture.go?
 ❯ 1. Yes
   2. Yes, allow all edits during this session (shift+tab)
   3. No

 Esc to cancel · Tab to amend
`

// Write fills dir with the demo: snapshot.json, transcripts under claude/,
// the waiting session's screen under panes/, made-up system stats, and a
// small git repository with uncommitted work for the Changes view. Times
// are relative to now. dir is emptied first.
func Write(dir string, now time.Time) error {
	now = now.UTC().Truncate(time.Second)
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "panes"), 0o755); err != nil {
		return err
	}
	var sessions []session
	add := func(i int, name, group, directory, status, convID string) {
		sessions = append(sessions, newSession(i, name, group, directory, status, convID, "claude"))
	}
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	write := func(c *conv, project string) (string, error) { return c.write(dir, project) }

	// 1. checkout-redesign: working, in a real git repository for Changes.
	c := newConv(ConvID(1), ago(14*time.Minute), "/home/dev/src/shopfront")
	c.prompt("Rebuild the checkout summary so tax and shipping update as soon as the address changes. Keep the existing tests passing and add one for the address change.", 20)
	c.title("Live tax and shipping in checkout summary")
	c.reply("I'll start with the summary component and the hook that computes totals.", 48000, 400, 12)
	c.tool("Read", record{"file_path": "/home/dev/src/shopfront/src/checkout/Summary.tsx"}, "export function Summary({ cart }) {\n  const totals = useTotals(cart)\n  ...", toolOpts{ctx: 52000})
	c.tool("Grep", record{"pattern": "useTotals", "path": "src"}, "src/checkout/Summary.tsx\nsrc/checkout/useTotals.ts", toolOpts{ctx: 54000})
	c.tool("Edit", record{
		"file_path":  "/home/dev/src/shopfront/src/checkout/useTotals.ts",
		"old_string": "export function useTotals(cart: Cart) {\n  return useMemo(() => compute(cart), [cart.id])\n}",
		"new_string": "export function useTotals(cart: Cart, address?: Address) {\n  return useMemo(() => compute(cart, address), [cart.id, cart.version, address?.postcode])\n}",
	}, "The file was updated.", toolOpts{ctx: 61000})
	c.reply("`useTotals` memoized on the cart id only, so a new address never recomputed tax or shipping. It now depends on the cart version and the postcode. Running the checkout tests:", 63000, 400, 12)
	testOut := " PASS  src/checkout/Summary.test.tsx\n PASS  src/checkout/useTotals.test.ts\n\nTests: 14 passed, 14 total"
	c.tool("Bash", record{"command": "npm test -- src/checkout", "description": "Run checkout tests"}, testOut, toolOpts{ctx: 66000, detail: record{"stdout": testOut, "stderr": ""}})
	c.reply("All 14 pass. Adding a test that changes the address and expects new totals.", 71000, 400, 12)
	c.tool("Write", record{"file_path": "/home/dev/src/shopfront/src/checkout/address.test.tsx", "content": "it('updates totals when the address changes', ...)"}, "File created.", toolOpts{ctx: 74000})
	if _, err := write(c, "-home-dev-src-shopfront"); err != nil {
		return err
	}
	add(1, "checkout-redesign", "shopfront", "${FIXTURES}/repo", "working", c.id)

	// 2. flaky-auth-test: waiting on a permission prompt. Its turn began a
	// minute ago, so once answered the live row counts from there.
	c = newConv(ConvID(2), ago(75*time.Second), "/home/dev/src/platform")
	c.prompt("The login integration test fails about one run in ten on CI. Find out why and fix it.", 20)
	c.title("Flaky login integration test")
	c.reply("Running it in a loop to reproduce.", 40000, 400, 12)
	c.tool("Bash", record{"command": "for i in $(seq 20); do go test ./auth -run TestLogin -count=1 || break; done", "description": "Loop the login test"},
		"--- FAIL: TestLogin (0.31s)\n    login_test.go:58: session expired before assertion", toolOpts{ctx: 45000, err: true})
	c.reply("Reproduced on run 7. The test session has a 300 ms TTL and the assertion sometimes runs after it expires. I want to raise the TTL in the test fixture only.", 52000, 400, 12)
	if _, err := write(c, "-home-dev-src-platform"); err != nil {
		return err
	}
	add(2, "flaky-auth-test", "platform", "/home/dev/src/platform", "waiting", c.id)
	if err := os.WriteFile(filepath.Join(dir, "panes", Waiting+".txt"), []byte(WaitingPane), 0o644); err != nil {
		return err
	}

	// 3. rate-limiter: finished.
	c = newConv(ConvID(3), ago(31*time.Minute), "/home/dev/src/platform")
	c.prompt("Add per-key rate limiting to the public API: 100 requests a minute, 429 with Retry-After when exceeded.", 20)
	c.title("Per-key rate limiting for the public API")
	c.reply("Looking at the middleware chain first.", 50000, 400, 12)
	c.tool("Read", record{"file_path": "/home/dev/src/platform/api/middleware.go"}, "func Chain(h http.Handler) http.Handler {...}", toolOpts{ctx: 55000})
	c.tool("Bash", record{"command": "go test ./api/...", "description": "Run API tests"}, "ok  \tplatform/api\t0.412s\nok  \tplatform/api/ratelimit\t0.088s", toolOpts{ctx: 88000})
	c.reply(`Rate limiting is in.

- **Limiter:** token bucket per API key in `+"`api/ratelimit`"+`, 100 requests a minute with a burst of 20.
- **Response:** 429 with `+"`Retry-After`"+` in seconds, and `+"`X-RateLimit-Remaining`"+` on every response.
- **Tests:** 9 new cases, including burst, refill and two keys not sharing a bucket.

One question: should internal service keys be exempt?`, 92000, 900, 12)
	c.duration(184000)
	if _, err := write(c, "-home-dev-src-platform"); err != nil {
		return err
	}
	add(3, "rate-limiter", "platform", "/home/dev/src/platform", "finished", c.id)

	// 4. release-orchestrator: working, with subagents.
	c = newConv(ConvID(4), ago(22*time.Minute), "/home/dev/src/platform")
	c.prompt("Prepare the 2.4 release: changelog, version bump, and a check that every migration has a rollback.", 20)
	c.title("Prepare the 2.4 release")
	c.reply("Splitting this into three parallel tasks.", 30000, 400, 12)
	c.tool("Agent", record{"description": "Write the 2.4 changelog", "subagent_type": "general-purpose", "prompt": "..."}, "Changelog drafted in CHANGELOG.md.", toolOpts{ctx: 34000, id: "toolu_rel1"})
	c.tool("Agent", record{"description": "Check migrations have rollbacks", "subagent_type": "general-purpose", "prompt": "...", "run_in_background": true}, "Started in the background.", toolOpts{ctx: 36000, id: "toolu_rel2"})
	c.reply("The changelog is drafted. The migration check is still running; I'll bump versions meanwhile.", 40000, 400, 12)
	d, err := write(c, "-home-dev-src-platform")
	if err != nil {
		return err
	}
	sub := filepath.Join(d, c.id, "subagents")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		return err
	}
	for _, a := range []struct {
		id, tool, desc string
		done           bool
	}{{"a1c0", "toolu_rel1", "Write the 2.4 changelog", true}, {"b2d0", "toolu_rel2", "Check migrations have rollbacks", false}} {
		meta, _ := json.Marshal(record{"agentType": "general-purpose", "isBuiltIn": true, "description": a.desc, "toolUseId": a.tool, "spawnDepth": 1})
		if err := os.WriteFile(filepath.Join(sub, "agent-"+a.id+".meta.json"), meta, 0o644); err != nil {
			return err
		}
		s := newConv(ConvID(40+len(a.id)), ago(20*time.Minute), "/home/dev/src/shopfront")
		s.prompt(a.desc+".", 20)
		s.tool("Bash", record{"command": "git log --oneline v2.3..HEAD | wc -l", "description": "Count commits"}, "47", toolOpts{ctx: 20000})
		if a.done {
			s.reply("Drafted CHANGELOG.md with 12 features, 9 fixes and 2 breaking changes.", 26000, 400, 12)
		} else {
			s.tool("Glob", record{"pattern": "migrations/*.up.sql"}, "migrations/0041_orders.up.sql\nmigrations/0042_refunds.up.sql", toolOpts{ctx: 24000})
		}
		for _, l := range s.lines {
			l["isSidechain"] = true
		}
		if err := writeLines(filepath.Join(sub, "agent-"+a.id+".jsonl"), s.lines); err != nil {
			return err
		}
	}
	add(4, "release-orchestrator", "platform", "/home/dev/src/platform", "working", c.id)

	// 5. docs-refresh: idle.
	c = newConv(ConvID(5), ago(3*time.Hour), "/home/dev/src/shopfront")
	c.prompt("Update the README quick start for the new config file format.", 20)
	c.title("README quick start for the new config")
	c.reply("Updated the quick start and the config reference. The old `.shopfrontrc` still loads, with a deprecation warning.", 35000, 400, 12)
	if _, err := write(c, "-home-dev-src-shopfront"); err != nil {
		return err
	}
	add(5, "docs-refresh", "shopfront", "/home/dev/src/shopfront", "idle", c.id)

	// 6. search-indexer: errored.
	c = newConv(ConvID(6), ago(48*time.Minute), "/home/dev/src/platform")
	c.prompt("Reindex the product catalog with the new analyzer and report how long it takes.", 20)
	c.title("Reindex the catalog with the new analyzer")
	c.reply("Starting the reindex job.", 30000, 400, 12)
	c.lines = append(c.lines, record{"type": "system", "uuid": "err-1", "subtype": "api_error", "level": "error", "timestamp": ts(c.t.Add(2 * time.Minute)), "content": "Request timed out"})
	if _, err := write(c, "-home-dev-src-platform"); err != nil {
		return err
	}
	add(6, "search-indexer", "platform", "/home/dev/src/platform", "errored", c.id)

	// 7. A Codex session.
	sessions = append(sessions, newSession(7, "migrate-config", "shopfront", "/home/dev/src/shopfront", "idle", "", "codex"))

	// History for the usage chart and the conversation list.
	topics := []string{"Cart badge count", "Image CDN headers", "Order export CSV", "Refund webhook retries", "Session cookie flags",
		"Search facets", "Admin audit log", "Slow product page query", "Coupon stacking rules", "Inventory sync job",
		"Email template preview", "Feature flag cleanup", "Postgres 17 upgrade notes", "Load test the checkout"}
	for i, topic := range topics {
		day := now.Add(-time.Duration(float64(24*time.Hour)*(1+float64(i)*1.4)) - time.Duration(i%5)*time.Hour)
		project := "platform"
		if i%2 == 1 {
			project = "shopfront"
		}
		h := newConv(ConvID(100+i), day, "/home/dev/src/"+project)
		if i%3 == 0 {
			h.model = "claude-sonnet-4-5"
		}
		h.prompt(topic+": take a look and fix what you find.", 20)
		h.title(topic)
		for turn := range 2 + i%3 {
			if turn > 0 {
				h.prompt("Looks good. Carry on with the next part.", float64(180+240*((i+turn)%4)))
			}
			for k := range 3 + (i*7+turn)%7 {
				h.reply("Working on it.", 40000+k*9000+i*3000, 300+k*40, 40)
			}
		}
		h.cost(float64(int((0.4+float64(i*37%23)/10)*100+0.5)) / 100)
		if _, err := write(h, "-home-dev-src-"+project); err != nil {
			return err
		}
	}

	snap := record{"version": 1, "manager_awake": true, "sessions": sessions, "terminals": []any{},
		"groups": []record{{"path": "shopfront", "archived": false, "sessions": 3}, {"path": "platform", "archived": false, "sessions": 4}}}
	if err := writeJSON(filepath.Join(dir, "snapshot.json"), snap); err != nil {
		return err
	}

	// Made-up system stats for the sidebar, so no real machine's numbers
	// show.
	const gb = 1 << 30
	stats := record{"supported": true, "at": ts(now), "cpu": record{"load": []float64{4.1, 3.6, 3.2}, "cores": 10},
		"memory": record{"total": 32 * gb, "available": 10 * gb, "pressure": "normal"},
		"swap":   record{"total": 2 * gb, "used": gb / 2},
		"disks":  []record{{"mount": "/", "total": 994 * gb, "free": 312 * gb}}}
	if err := writeJSON(filepath.Join(dir, "stats.json"), stats); err != nil {
		return err
	}
	return writeRepo(filepath.Join(dir, "repo"))
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// writeRepo makes a repository with one commit and uncommitted work, for
// the Changes view. Without git it is skipped, and Changes says the
// session is not in a repository. The user's git configuration cannot run
// hooks or ask to sign the commit.
func writeRepo(repo string) error {
	if _, err := exec.LookPath("git"); err != nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Join(repo, "src", "checkout"), 0o755); err != nil {
		return err
	}
	g := func(args ...string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		base := []string{"-C", repo, "-c", "user.name=demo", "-c", "user.email=demo@example.com", "-c", "init.defaultBranch=main",
			"-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "-c", "core.fsmonitor=false"}
		cmd := exec.CommandContext(ctx, "git", append(base, args...)...)
		cmd.Env = gitEnv()
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("demo repository: git %s: %v: %s", args[0], err, out)
		}
		return nil
	}
	file := filepath.Join(repo, "src", "checkout", "useTotals.ts")
	if err := g("init", "-q"); err != nil {
		return err
	}
	if err := os.WriteFile(file, []byte(`import { useMemo } from 'react'
import { compute, type Cart } from './compute'

export function useTotals(cart: Cart) {
  return useMemo(() => compute(cart), [cart.id])
}
`), 0o644); err != nil {
		return err
	}
	if err := g("add", "."); err != nil {
		return err
	}
	if err := g("commit", "-q", "--no-verify", "-m", "checkout summary"); err != nil {
		return err
	}
	if err := os.WriteFile(file, []byte(`import { useMemo } from 'react'
import { compute, type Address, type Cart } from './compute'

// Totals depend on the address: tax and shipping change with the postcode.
export function useTotals(cart: Cart, address?: Address) {
  return useMemo(() => compute(cart, address), [cart.id, cart.version, address?.postcode])
}
`), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(repo, "src", "checkout", "address.test.tsx"), []byte("it('updates totals when the address changes', () => {})\n"), 0o644)
}

// gitEnv is the environment without GIT_ variables, which could point git
// at another repository or configuration.
func gitEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	return env
}
