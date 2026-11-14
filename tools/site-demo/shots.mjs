// Takes the site's feature screenshots from made-up sessions: the overview,
// a waiting question, line comments on a session's changes, and usage, at
// 1440×900 in the dark theme. Nothing real is shown: the sessions and the
// system stats come from gen.py next to this file.
//
//   make build
//   python3 tools/site-demo/gen.py /tmp/site-demo/desk
//   node tools/site-demo/shots.mjs /tmp/site-demo/desk site/img
import { spawn } from 'node:child_process'
import { createRequire } from 'node:module'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const root = join(here, '..', '..')
const require = createRequire(join(root, 'web', 'package.json'))
const { chromium } = require('@playwright/test')

const [fixtures, out] = process.argv.slice(2)
if (!fixtures || !out) {
  console.error('usage: node tools/site-demo/shots.mjs <fixture-dir> <out-dir>')
  process.exit(2)
}
const port = 7394
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

async function main() {
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
  const token = url.split('#t=')[1]
  const base = `http://127.0.0.1:${String(port)}`
  const browser = await chromium.launch()
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, colorScheme: 'dark' })
  // A returning user, and the usual "Turn on notifications" link rather
  // than a headless browser's blocked warning.
  await context.addInitScript(() => {
    localStorage.setItem('agent-desk:welcomed', '1')
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
  const shot = async (name) => {
    await sleep(600)
    await page.mouse.move(0, 899)
    await page.screenshot({ path: join(out, name) })
    console.log(`wrote ${join(out, name)}`)
  }
  try {
    await page.goto(`${base}/#t=${token}`)
    await page.getByRole('region', { name: 'System' }).waitFor()
    await page.locator('.card').first().waitFor()
    await shot('overview.png')

    await page.goto(`${base}/s/d0000002#t=${token}`)
    await page.locator('.live-question').waitFor()
    await shot('waiting.png')

    await page.goto(`${base}/s/d0000001#t=${token}`)
    await page.getByRole('button', { name: 'Changes', exact: true }).click()
    const panel = page.getByRole('complementary', { name: 'Changes' })
    await panel.locator('tr.diff-add').filter({ hasText: 'Totals depend on the address' }).click()
    const box = panel.getByRole('textbox', { name: 'Review comment' })
    await box.fill('Say why: the API rejects partial postcodes, so totals wait for a full one.')
    await box.press('Enter')
    await panel.locator('.review-comment').waitFor()
    await shot('changes.png')

    await page.goto(`${base}/usage#t=${token}`)
    await page.getByRole('radio', { name: '30 days' }).click()
    await page.getByText('Tokens per day').waitFor()
    await shot('usage.png')
  } finally {
    await browser.close()
    server.kill()
  }
}

main().catch((e) => {
  console.error(e)
  process.exit(1)
})
