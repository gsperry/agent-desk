// Records short clips for social posts, one problem each, from
// agent-desk --demo: answering a waiting agent, reading and reviewing a
// session, the whole fleet at once, and answering from a phone. Nothing
// real is shown. Each clip starts its own demo, so the waiting session is
// fresh every time.
//
//   make build
//   node tools/site-demo/clips.mjs <out-dir> [clip...]
//
// Clips: answer, review, fleet, phone. They come out as .webm; convert
// for X or Reddit with ffmpeg -i in.webm -c:v libx264 -pix_fmt yuv420p out.mp4
import { spawn } from 'node:child_process'
import { copyFileSync, mkdirSync, rmSync } from 'node:fs'
import { createRequire } from 'node:module'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { tmpdir } from 'node:os'

const here = dirname(fileURLToPath(import.meta.url))
const root = join(here, '..', '..')
const require = createRequire(join(root, 'web', 'package.json'))
const { chromium } = require('@playwright/test')

const [out, ...only] = process.argv.slice(2)
if (!out) {
  console.error('usage: node tools/site-demo/clips.mjs <out-dir> [answer|review|fleet|phone...]')
  process.exit(2)
}
mkdirSync(out, { recursive: true })
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

// A tap ring for the phone clip, where there is no pointer.
const taps = () => {
  addEventListener('pointerdown', (e) => {
    const ring = document.createElement('div')
    ring.style.cssText = `position:fixed;left:${String(e.clientX)}px;top:${String(e.clientY)}px;width:34px;height:34px;margin:-17px 0 0 -17px;border-radius:50%;border:3px solid rgba(255,255,255,.85);pointer-events:none;z-index:99999;transition:transform .35s,opacity .35s`
    document.body.appendChild(ring)
    requestAnimationFrame(() => {
      ring.style.transform = 'scale(1.6)'
      ring.style.opacity = '0'
    })
    setTimeout(() => ring.remove(), 400)
  })
}

async function demoServer(port) {
  const server = spawn(join(root, 'bin', 'agent-desk'), ['--demo', '--no-open', '--port', String(port)], { stdio: ['ignore', 'pipe', 'inherit'] })
  const url = await new Promise((resolve, reject) => {
    server.stdout.on('data', (b) => {
      const m = /Open: (\S+)/.exec(String(b))
      if (m) resolve(m[1])
    })
    server.on('exit', () => {
      reject(new Error('demo exited'))
    })
  })
  return { server, url }
}

async function record(name, { size, mobile = false }, script) {
  const port = 7380 + Math.floor(Math.random() * 10)
  const { server, url } = await demoServer(port)
  const videoDir = join(tmpdir(), `agent-desk-clip-${name}`)
  rmSync(videoDir, { recursive: true, force: true })
  const browser = await chromium.launch()
  const context = await browser.newContext({
    viewport: size,
    colorScheme: 'dark',
    recordVideo: { dir: videoDir, size },
    ...(mobile ? { isMobile: true, hasTouch: true, deviceScaleFactor: 1 } : {}),
  })
  await context.addInitScript(mobile ? taps : pointer)
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
  const glide = async (locator, click = true) => {
    const b = await locator.boundingBox()
    if (!b) throw new Error('nothing to point at')
    await page.mouse.move(b.x + b.width / 2, b.y + b.height / 2, { steps: 25 })
    await sleep(250)
    if (click) await page.mouse.click(b.x + b.width / 2, b.y + b.height / 2)
  }
  try {
    await page.mouse.move(size.width / 2, size.height / 2)
    await page.goto(url)
    await page.locator('.card, .sidebar').first().waitFor()
    await script(page, glide)
  } finally {
    const video = page.video()
    await context.close()
    await browser.close()
    server.kill()
    if (video) {
      const file = join(out, `${name}.webm`)
      copyFileSync(await video.path(), file)
      console.log(`wrote ${file}`)
    }
  }
}

const wide = { size: { width: 1280, height: 720 } }

const clips = {
  // A waiting agent, answered with one click, carries on.
  async answer() {
    await record('answer', wide, async (page, glide) => {
      await sleep(1500)
      await glide(page.getByRole('navigation', { name: 'Sessions' }).getByText('flaky-auth-test'))
      const pane = page.getByRole('region', { name: 'Terminal' })
      await pane.locator('.live-question').waitFor()
      await sleep(1800)
      await glide(pane.getByRole('button', { name: /^1\s+Yes/ }))
      await page.getByRole('status', { name: 'Agent activity' }).waitFor()
      await page.getByText('passed 20 runs in a row').waitFor({ timeout: 20000 })
      await sleep(2500)
    })
  },

  // A session read as a document, its diff, and a line comment sent back.
  async review() {
    await record('review', wide, async (page, glide) => {
      await sleep(1000)
      await glide(page.getByRole('navigation', { name: 'Sessions' }).getByText('checkout-redesign'))
      await page.getByText('All 14 pass').waitFor()
      await sleep(1500)
      await glide(page.getByRole('button', { name: 'Changes', exact: true }))
      const panel = page.getByRole('complementary', { name: 'Changes' })
      const line = panel.locator('tr.diff-add').filter({ hasText: 'Totals depend on the address' })
      await line.waitFor()
      await sleep(1200)
      await glide(line)
      const box = panel.getByRole('textbox', { name: 'Review comment' })
      await box.pressSequentially('Say why: the API rejects partial postcodes.', { delay: 45 })
      await box.press('Enter')
      await sleep(800)
      await glide(panel.getByRole('button', { name: 'Send review' }))
      await page.locator('.card.user').filter({ hasText: 'Review comments on your changes' }).waitFor()
      await sleep(1200)
      await page.getByText('This is a demo session').last().waitFor({ timeout: 15000 })
      await sleep(1500)
    })
  },

  // Every session on one screen; j jumps to the next that needs you.
  async fleet() {
    await record('fleet', wide, async (page, glide) => {
      await sleep(2000)
      const cards = page.locator('.card')
      for (const i of [0, 1, 2, 3]) {
        await glide(cards.nth(i), false)
        await sleep(500)
      }
      await glide(page.getByRole('button', { name: /Needs you/ }))
      await sleep(1500)
      await page.keyboard.press('j')
      await sleep(1500)
      await page.keyboard.press('j')
      await sleep(1500)
      await page.keyboard.press('Meta+k')
      await sleep(600)
      await page.keyboard.type('rate', { delay: 120 })
      await sleep(900)
      await page.keyboard.press('Enter')
      await sleep(2200)
    })
  },

  // Answering a waiting agent from a phone.
  async phone() {
    await record('phone', { size: { width: 390, height: 844 }, mobile: true }, async (page) => {
      await sleep(1500)
      await page.getByRole('navigation', { name: 'Sessions' }).getByText('flaky-auth-test').tap()
      const pane = page.getByRole('region', { name: 'Terminal' })
      await pane.locator('.live-question').waitFor()
      await pane.scrollIntoViewIfNeeded()
      await sleep(2000)
      await pane.getByRole('button', { name: /^1\s+Yes/ }).tap()
      await page.getByText('passed 20 runs in a row').waitFor({ timeout: 20000 })
      await sleep(2500)
    })
  },
}

for (const name of only.length ? only : Object.keys(clips)) {
  if (!clips[name]) throw new Error(`no clip ${name}`)
  await clips[name]()
}
