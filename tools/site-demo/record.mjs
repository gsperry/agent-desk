// Records the demo video on agent-desk.app from made-up sessions: the
// overview, a waiting question answered with one click, the agent working
// on with the live activity row, its reply, and a reply sent with Enter.
// Nothing real is shown: the sessions come from gen.py next to this file.
//
//   make build
//   python3 tools/site-demo/gen.py /tmp/site-demo/desk
//   node tools/site-demo/record.mjs /tmp/site-demo/desk site/img/demo.webm
import { spawn } from 'node:child_process'
import { appendFileSync, copyFileSync, readFileSync, readdirSync, writeFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const root = join(here, '..', '..')
const require = createRequire(join(root, 'web', 'package.json'))
const { chromium } = require('@playwright/test')

const [fixtures, out] = process.argv.slice(2)
if (!fixtures || !out) {
  console.error('usage: node tools/site-demo/record.mjs <fixture-dir> <out.webm>')
  process.exit(2)
}
const port = 7393
const size = { width: 1280, height: 800 }
const session = 'd0000002'
const conversation = '00000002-5e5e-4d00-8000-000000000002'

const transcriptPath = (() => {
  const projects = join(fixtures, 'claude', 'projects')
  for (const dir of readdirSync(projects)) {
    const p = join(projects, dir, `${conversation}.jsonl`)
    try {
      readFileSync(p)
      return p
    } catch {
      // not in this project folder
    }
  }
  throw new Error('demo transcript not found')
})()

// The agent's next steps, appended as the video runs.
let seq = 100
const now = () => new Date().toISOString()
const record = (r) => {
  appendFileSync(transcriptPath, JSON.stringify({ ...r, uuid: `00000002-${String(seq++).padStart(4, '0')}`, timestamp: now() }) + '\n')
}
const assistant = (content, tokens) =>
  record({ type: 'assistant', message: { id: `msg_demo_${String(seq)}`, model: 'claude-opus-4-1', role: 'assistant', content, usage: { input_tokens: 400, cache_read_input_tokens: 52000, output_tokens: tokens } } })
const result = (id, content) => record({ type: 'user', message: { role: 'user', content: [{ type: 'tool_result', tool_use_id: id, content }] } })

const setStatus = (status) => {
  const p = join(fixtures, 'snapshot.json')
  const snap = JSON.parse(readFileSync(p, 'utf8'))
  for (const s of snap.sessions) if (s.id === session) s.status = status
  writeFileSync(p, JSON.stringify(snap, null, 2))
}
const idlePane = () => {
  const rule = '─'.repeat(100)
  writeFileSync(join(fixtures, 'panes', `${session}.txt`), `${rule}\n❯ \n${rule}\n  ⏵⏵ accept edits on\n`)
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

// A pointer drawn into the page, since a headless recording shows none.
const pointer = () => {
  const dot = document.createElement('div')
  dot.style.cssText = 'position:fixed;left:0;top:0;width:18px;height:18px;margin:-9px 0 0 -9px;border-radius:50%;background:rgba(255,255,255,.85);box-shadow:0 0 0 2px rgba(0,0,0,.45);pointer-events:none;z-index:99999;transition:transform .12s'
  addEventListener('mousemove', (e) => {
    dot.style.left = `${e.clientX}px`
    dot.style.top = `${e.clientY}px`
  })
  addEventListener('mousedown', () => {
    dot.style.transform = 'scale(.6)'
  })
  addEventListener('mouseup', () => {
    dot.style.transform = ''
  })
  addEventListener('DOMContentLoaded', () => {
    document.body.appendChild(dot)
  })
}

// freshPrompt moves the demo session's last prompt to a few seconds ago,
// so the live row counts the turn in seconds, as it would live.
const freshPrompt = () => {
  const lines = readFileSync(transcriptPath, 'utf8').split('\n')
  let last = -1
  lines.forEach((l, i) => {
    if (l.includes('"type": "user"') && !l.includes('tool_result')) last = i
  })
  if (last < 0) return
  const r = JSON.parse(lines[last])
  r.timestamp = new Date(Date.now() - 20_000).toISOString()
  lines[last] = JSON.stringify(r)
  writeFileSync(transcriptPath, lines.join('\n'))
}

async function main() {
  freshPrompt()
  const server = spawn(join(root, 'bin', 'agent-desk'), ['--fixtures', fixtures, '--no-open', '--port', String(port)], { stdio: ['ignore', 'pipe', 'inherit'] })
  const url = await new Promise((resolve, reject) => {
    server.stdout.on('data', (b) => {
      const m = /Open: (\S+)/.exec(String(b))
      if (m) resolve(m[1])
    })
    server.on('exit', () => {
      reject(new Error('server exited'))
    })
  })
  const browser = await chromium.launch()
  const context = await browser.newContext({ viewport: size, colorScheme: 'dark', recordVideo: { dir: join(fixtures, 'video'), size } })
  await context.addInitScript(pointer)
  // A returning user: the overview, not the first-run setup checklist.
  await context.addInitScript(() => {
    localStorage.setItem('agent-desk:welcomed', '1')
  })
  // A headless browser reports notifications as blocked; show the usual
  // "Turn on notifications" link instead of that warning.
  await context.addInitScript(() => {
    Object.defineProperty(window, 'Notification', {
      value: class {
        static permission = 'default'
        static requestPermission() {
          return Promise.resolve('default')
        }
      },
    })
  })
  const page = await context.newPage()
  // DEMO_SHOTS=<dir> saves a screenshot at each stage, to check a run.
  const shots = process.env.DEMO_SHOTS
  let stage = 0
  const shot = async () => {
    if (shots) await page.screenshot({ path: join(shots, `stage-${String(++stage)}.png`) })
  }
  const glide = async (locator, click = true) => {
    const b = await locator.boundingBox()
    if (!b) throw new Error('nothing to point at')
    await page.mouse.move(b.x + b.width / 2, b.y + b.height / 2, { steps: 25 })
    await sleep(250)
    if (click) await page.mouse.click(b.x + b.width / 2, b.y + b.height / 2)
  }
  try {
    await page.mouse.move(640, 400)
    await page.goto(url)
    await page.getByText('Overview').first().waitFor()
    await sleep(2200)
    await shot()

    // A session needs you: open it.
    await glide(page.getByRole('navigation', { name: 'Sessions' }).getByText('flaky-auth-test'))
    const pane = page.getByRole('region', { name: 'Terminal' })
    await pane.locator('.live-question').waitFor()
    await sleep(2200)
    await shot()

    // Answer it with one click.
    await glide(pane.getByRole('button', { name: /^1\s+Yes/ }))
    idlePane()
    setStatus('working')
    await sleep(600)
    assistant([{ type: 'tool_use', id: 'toolu_demo1', name: 'Edit', input: { file_path: '/home/dev/src/platform/auth/testdata/fixture.go', old_string: 'TTL: 300 * time.Millisecond,', new_string: 'TTL: 5 * time.Second,' } }], 180)
    await sleep(1600)
    result('toolu_demo1', 'The file auth/testdata/fixture.go has been updated.')
    assistant([{ type: 'tool_use', id: 'toolu_demo2', name: 'Bash', input: { command: 'go test ./auth -run TestLogin -count=20', description: 'Run the login test 20 times' } }], 260)
    await page.getByRole('status', { name: 'Agent activity' }).waitFor()
    await sleep(3200)
    await shot()
    result('toolu_demo2', 'ok  \texample.com/platform/auth\t6.214s')
    assistant([{ type: 'text', text: 'Raised the session TTL in the test fixture from 300 ms to 5 s. The login test passed 20 runs in a row; production code is unchanged.' }], 410)
    record({ type: 'system', subtype: 'turn_duration', durationMs: 9000 })
    setStatus('finished')
    await sleep(2600)
    await shot()

    // Reply, and Enter sends it.
    const box = page.getByRole('textbox', { name: /Reply to/ })
    await glide(box)
    await box.pressSequentially('Thanks. Open a PR with this.', { delay: 55 })
    await sleep(500)
    await box.press('Enter')
    await shot()
    await sleep(2600)
  } finally {
    const video = page.video()
    await context.close()
    await browser.close()
    server.kill()
    if (video) copyFileSync(await video.path(), out)
  }
  console.log(`wrote ${out}`)
}

main().catch((e) => {
  console.error(e)
  process.exit(1)
})
