"""Generates a fictional Agent Desk fixture set for website screenshots."""
import json
import os
import shutil
import subprocess
import sys
from datetime import datetime, timedelta, timezone

OUT = sys.argv[1]
NOW = datetime.now(timezone.utc).replace(microsecond=0)


def ts(t):
    return t.strftime("%Y-%m-%dT%H:%M:%S.000Z")


class Conv:
    def __init__(self, cid, start, model="claude-opus-4-1", cwd="/home/dev/src/shopfront"):
        self.cid, self.t, self.lines, self.n, self.model, self.cwd = cid, start, [], 0, model, cwd

    def _uuid(self):
        self.n += 1
        return f"{self.cid[:8]}-{self.n:04d}"

    def tick(self, s):
        self.t += timedelta(seconds=s)

    def prompt(self, text, gap=20):
        self.tick(gap)
        self.lines.append({"type": "user", "uuid": self._uuid(), "timestamp": ts(self.t), "cwd": self.cwd,
                           "gitBranch": "main", "promptSource": "typed", "origin": {"kind": "human"},
                           "message": {"role": "user", "content": text}})

    def reply(self, text, ctx=60000, out=400, gap=12):
        self.tick(gap)
        self.lines.append({"type": "assistant", "uuid": self._uuid(), "timestamp": ts(self.t),
                           "message": {"id": "msg_" + self._uuid(), "model": self.model, "role": "assistant",
                                       "content": [{"type": "text", "text": text}],
                                       "usage": {"input_tokens": 1200, "output_tokens": out,
                                                 "cache_read_input_tokens": ctx - 1200, "cache_creation_input_tokens": 0}}})

    def tool(self, name, inp, result, ctx=60000, gap=6, detail=None, error=False, tid=None):
        self.tick(gap)
        tid = tid or "toolu_" + self._uuid().replace("-", "")
        self.lines.append({"type": "assistant", "uuid": self._uuid(), "timestamp": ts(self.t),
                           "message": {"id": "msg_" + self._uuid(), "model": self.model, "role": "assistant",
                                       "content": [{"type": "tool_use", "id": tid, "name": name, "input": inp}],
                                       "usage": {"input_tokens": 900, "output_tokens": 120,
                                                 "cache_read_input_tokens": ctx - 900, "cache_creation_input_tokens": 0}}})
        self.tick(3)
        rec = {"type": "user", "uuid": self._uuid(), "timestamp": ts(self.t),
               "message": {"role": "user", "content": [{"type": "tool_result", "tool_use_id": tid, "content": result,
                                                        **({"is_error": True} if error else {})}]}}
        if detail:
            rec["toolUseResult"] = detail
        self.lines.append(rec)
        return tid

    def cost(self, usd):
        self.lines.append({"type": "cost-state", "startTime": 1, "totalCostUSD": usd})

    def title(self, text):
        self.lines.append({"type": "ai-title", "aiTitle": text, "sessionId": self.cid})

    def duration(self, ms):
        self.lines.append({"type": "system", "uuid": self._uuid(), "subtype": "turn_duration", "durationMs": ms})

    def write(self, project):
        d = os.path.join(OUT, "claude", "projects", project)
        os.makedirs(d, exist_ok=True)
        with open(os.path.join(d, self.cid + ".jsonl"), "w") as f:
            for l in self.lines:
                f.write(json.dumps(l) + "\n")
        return d


def cid(n):
    return f"{n:08x}-5e5e-4d00-8000-{n:012x}"


shutil.rmtree(OUT, ignore_errors=True)
os.makedirs(os.path.join(OUT, "panes"))
sessions = []


def session(i, name, group, directory, status, conv=None, tool="claude"):
    sessions.append({"id": f"d000{i:04d}", "name": name, "tool": tool, "group": group, "directory": directory,
                     "status": status, "running": status != "dead", "archived": False,
                     "agent_session_id": conv or "", "self": False})


# 1. checkout-redesign: working, in a real git repo for the Changes view.
c = Conv(cid(1), NOW - timedelta(minutes=14))
c.prompt("Rebuild the checkout summary so tax and shipping update as soon as the address changes. Keep the existing tests passing and add one for the address change.")
c.title("Live tax and shipping in checkout summary")
c.reply("I'll start with the summary component and the hook that computes totals.", ctx=48000)
c.tool("Read", {"file_path": "/home/dev/src/shopfront/src/checkout/Summary.tsx"}, "export function Summary({ cart }) {\n  const totals = useTotals(cart)\n  ...", ctx=52000)
c.tool("Grep", {"pattern": "useTotals", "path": "src"}, "src/checkout/Summary.tsx\nsrc/checkout/useTotals.ts", ctx=54000)
c.tool("Edit", {"file_path": "/home/dev/src/shopfront/src/checkout/useTotals.ts",
                "old_string": "export function useTotals(cart: Cart) {\n  return useMemo(() => compute(cart), [cart.id])\n}",
                "new_string": "export function useTotals(cart: Cart, address?: Address) {\n  return useMemo(() => compute(cart, address), [cart.id, cart.version, address?.postcode])\n}"},
       "The file was updated.", ctx=61000)
c.reply("`useTotals` memoized on the cart id only, so a new address never recomputed tax or shipping. It now depends on the cart version and the postcode. Running the checkout tests:", ctx=63000)
c.tool("Bash", {"command": "npm test -- src/checkout", "description": "Run checkout tests"},
       " PASS  src/checkout/Summary.test.tsx\n PASS  src/checkout/useTotals.test.ts\n\nTests: 14 passed, 14 total", ctx=66000,
       detail={"stdout": " PASS  src/checkout/Summary.test.tsx\n PASS  src/checkout/useTotals.test.ts\n\nTests: 14 passed, 14 total", "stderr": ""})
c.reply("All 14 pass. Adding a test that changes the address and expects new totals.", ctx=71000)
c.tool("Write", {"file_path": "/home/dev/src/shopfront/src/checkout/address.test.tsx", "content": "it('updates totals when the address changes', ...)"}, "File created.", ctx=74000)
c.write("-home-dev-src-shopfront")
session(1, "checkout-redesign", "shopfront", "${FIXTURES}/repo", "working", c.cid)

# 2. flaky-auth-test: waiting on a permission prompt.
c = Conv(cid(2), cwd='/home/dev/src/platform', start=NOW - timedelta(minutes=9))
c.prompt("The login integration test fails about one run in ten on CI. Find out why and fix it.")
c.title("Flaky login integration test")
c.reply("Running it in a loop to reproduce.", ctx=40000)
c.tool("Bash", {"command": "for i in $(seq 20); do go test ./auth -run TestLogin -count=1 || break; done", "description": "Loop the login test"},
       "--- FAIL: TestLogin (0.31s)\n    login_test.go:58: session expired before assertion", ctx=45000, error=True)
c.reply("Reproduced on run 7. The test session has a 300 ms TTL and the assertion sometimes runs after it expires. I want to raise the TTL in the test fixture only.", ctx=52000)
c.write("-home-dev-src-platform")
session(2, "flaky-auth-test", "platform", "/home/dev/src/platform", "waiting", c.cid)
with open(os.path.join(OUT, "panes", "d0000002.txt"), "w") as f:
    f.write("""● Reproduced on run 7. The test session has a 300 ms TTL and the
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
""")

# 3. rate-limiter: finished.
c = Conv(cid(3), cwd='/home/dev/src/platform', start=NOW - timedelta(minutes=31))
c.prompt("Add per-key rate limiting to the public API: 100 requests a minute, 429 with Retry-After when exceeded.")
c.title("Per-key rate limiting for the public API")
c.reply("Looking at the middleware chain first.", ctx=50000)
c.tool("Read", {"file_path": "/home/dev/src/platform/api/middleware.go"}, "func Chain(h http.Handler) http.Handler {...}", ctx=55000)
c.tool("Bash", {"command": "go test ./api/...", "description": "Run API tests"}, "ok  \tplatform/api\t0.412s\nok  \tplatform/api/ratelimit\t0.088s", ctx=88000)
c.reply("""Rate limiting is in.

- **Limiter:** token bucket per API key in `api/ratelimit`, 100 requests a minute with a burst of 20.
- **Response:** 429 with `Retry-After` in seconds, and `X-RateLimit-Remaining` on every response.
- **Tests:** 9 new cases, including burst, refill and two keys not sharing a bucket.

One question: should internal service keys be exempt?""", ctx=92000, out=900)
c.duration(184000)
c.write("-home-dev-src-platform")
session(3, "rate-limiter", "platform", "/home/dev/src/platform", "finished", c.cid)

# 4. release-orchestrator: working, with subagents.
c = Conv(cid(4), cwd='/home/dev/src/platform', start=NOW - timedelta(minutes=22))
c.prompt("Prepare the 2.4 release: changelog, version bump, and a check that every migration has a rollback.")
c.title("Prepare the 2.4 release")
c.reply("Splitting this into three parallel tasks.", ctx=30000)
t1 = c.tool("Agent", {"description": "Write the 2.4 changelog", "subagent_type": "general-purpose", "prompt": "..."}, "Changelog drafted in CHANGELOG.md.", ctx=34000, tid="toolu_rel1")
t2 = c.tool("Agent", {"description": "Check migrations have rollbacks", "subagent_type": "general-purpose", "prompt": "...", "run_in_background": True}, "Started in the background.", ctx=36000, tid="toolu_rel2")
c.reply("The changelog is drafted. The migration check is still running; I'll bump versions meanwhile.", ctx=40000)
d = c.write("-home-dev-src-platform")
sub = os.path.join(d, c.cid, "subagents")
os.makedirs(sub)
for aid, tid, desc, done in (("a1c0", "toolu_rel1", "Write the 2.4 changelog", True), ("b2d0", "toolu_rel2", "Check migrations have rollbacks", False)):
    json.dump({"agentType": "general-purpose", "isBuiltIn": True, "description": desc, "toolUseId": tid, "spawnDepth": 1}, open(os.path.join(sub, f"agent-{aid}.meta.json"), "w"))
    s = Conv(cid(40 + len(aid)), NOW - timedelta(minutes=20))
    s.prompt(desc + ".")
    s.tool("Bash", {"command": "git log --oneline v2.3..HEAD | wc -l", "description": "Count commits"}, "47", ctx=20000)
    if done:
        s.reply("Drafted CHANGELOG.md with 12 features, 9 fixes and 2 breaking changes.", ctx=26000)
    else:
        s.tool("Glob", {"pattern": "migrations/*.up.sql"}, "migrations/0041_orders.up.sql\nmigrations/0042_refunds.up.sql", ctx=24000)
    with open(os.path.join(sub, f"agent-{aid}.jsonl"), "w") as f:
        for l in s.lines:
            l["isSidechain"] = True
            f.write(json.dumps(l) + "\n")
session(4, "release-orchestrator", "platform", "/home/dev/src/platform", "working", c.cid)

# 5. docs-refresh: idle.
c = Conv(cid(5), NOW - timedelta(hours=3))
c.prompt("Update the README quick start for the new config file format.")
c.title("README quick start for the new config")
c.reply("Updated the quick start and the config reference. The old `.shopfrontrc` still loads, with a deprecation warning.", ctx=35000)
c.write("-home-dev-src-shopfront")
session(5, "docs-refresh", "shopfront", "/home/dev/src/shopfront", "idle", c.cid)

# 6. search-indexer: errored.
c = Conv(cid(6), cwd='/home/dev/src/platform', start=NOW - timedelta(minutes=48))
c.prompt("Reindex the product catalog with the new analyzer and report how long it takes.")
c.title("Reindex the catalog with the new analyzer")
c.reply("Starting the reindex job.", ctx=30000)
c.lines.append({"type": "system", "uuid": "err-1", "subtype": "api_error", "level": "error", "timestamp": ts(c.t + timedelta(minutes=2)), "content": "Request timed out"})
c.write("-home-dev-src-platform")
session(6, "search-indexer", "platform", "/home/dev/src/platform", "errored", c.cid)

# 7. A codex session.
session(7, "migrate-config", "shopfront", "/home/dev/src/shopfront", "idle", "", tool="codex")

# History for the usage chart and the conversations list.
topics = ["Cart badge count", "Image CDN headers", "Order export CSV", "Refund webhook retries", "Session cookie flags",
          "Search facets", "Admin audit log", "Slow product page query", "Coupon stacking rules", "Inventory sync job",
          "Email template preview", "Feature flag cleanup", "Postgres 17 upgrade notes", "Load test the checkout"]
for i, topic in enumerate(topics):
    day = NOW - timedelta(days=1 + i * 1.4, hours=i % 5)
    project = "shopfront" if i % 2 else "platform"
    h = Conv(cid(100 + i), day, model="claude-sonnet-4-5" if i % 3 == 0 else "claude-opus-4-1", cwd=f"/home/dev/src/{project}")
    h.prompt(f"{topic}: take a look and fix what you find.")
    h.title(topic)
    for turn in range(2 + i % 3):
        if turn:
            h.prompt("Looks good. Carry on with the next part.", gap=180 + 240 * ((i + turn) % 4))
        for k in range(3 + (i * 7 + turn) % 7):
            h.reply("Working on it.", ctx=40000 + k * 9000 + i * 3000, out=300 + k * 40, gap=40)
    h.cost(round(0.4 + (i * 37 % 23) / 10, 2))
    h.write(f"-home-dev-src-{project}")

json.dump({"version": 1, "manager_awake": True, "sessions": sessions, "terminals": [],
           "groups": [{"path": "shopfront", "archived": False, "sessions": 3}, {"path": "platform", "archived": False, "sessions": 4}]},
          open(os.path.join(OUT, "snapshot.json"), "w"), indent=1)

# A repository with uncommitted work for the Changes view.
repo = os.path.join(OUT, "repo")
os.makedirs(os.path.join(repo, "src", "checkout"))
def g(*a):
    subprocess.run(["git", "-C", repo, "-c", "user.name=demo", "-c", "user.email=demo@example.com", "-c", "init.defaultBranch=main", *a], check=True, capture_output=True)
g("init", "-q")
open(os.path.join(repo, "src/checkout/useTotals.ts"), "w").write("""import { useMemo } from 'react'
import { compute, type Cart } from './compute'

export function useTotals(cart: Cart) {
  return useMemo(() => compute(cart), [cart.id])
}
""")
g("add", ".")
g("commit", "-q", "-m", "checkout summary")
open(os.path.join(repo, "src/checkout/useTotals.ts"), "w").write("""import { useMemo } from 'react'
import { compute, type Address, type Cart } from './compute'

// Totals depend on the address: tax and shipping change with the postcode.
export function useTotals(cart: Cart, address?: Address) {
  return useMemo(() => compute(cart, address), [cart.id, cart.version, address?.postcode])
}
""")
open(os.path.join(repo, "src/checkout/address.test.tsx"), "w").write("it('updates totals when the address changes', () => {})\n")
print("ok", OUT)
