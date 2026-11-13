// Renders site/img/og.png, the 1200×630 preview shown when agent-desk.app
// is shared, from the site's colors and a screenshot of the made-up demo.
//
//   node tools/site-demo/og.mjs
import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..')
const require = createRequire(join(root, 'web', 'package.json'))
const { chromium } = require('@playwright/test')

const shot = readFileSync(join(root, 'site', 'img', 'waiting.png')).toString('base64')
const mark = readFileSync(join(root, 'site', 'favicon.svg')).toString('base64')

const html = `<!doctype html><html><head><style>
  body { margin: 0; width: 1200px; height: 630px; overflow: hidden; background: #0d1015; color: #dfe4ec;
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Inter, Roboto, sans-serif; }
  .glow { position: absolute; inset: 0; background: radial-gradient(800px 420px at 85% 15%, rgba(120,166,220,.16), transparent 70%); }
  .text { position: absolute; left: 64px; top: 70px; width: 470px; }
  .brand { display: flex; align-items: center; gap: 14px; font-size: 30px; font-weight: 700; letter-spacing: -.01em; }
  .brand img { width: 44px; height: 44px; }
  h1 { margin: 44px 0 20px; font-size: 54px; line-height: 1.05; letter-spacing: -.03em; font-weight: 800; }
  p { margin: 0; font-size: 24px; line-height: 1.4; color: #9aa4b2; }
  .cmd { position: absolute; left: 64px; bottom: 64px; font: 19px ui-monospace, 'SF Mono', Menlo, monospace;
    color: #dfe4ec; background: #161b23; border: 1px solid #252c37; border-radius: 10px; padding: 14px 18px; }
  .cmd span { color: #78a6dc; }
  .shot { position: absolute; left: 580px; top: 64px; width: 760px; border: 1px solid #252c37; border-radius: 12px;
    box-shadow: 0 30px 80px -20px rgba(0,0,0,.8); }
</style></head><body>
  <div class="glow"></div>
  <div class="text">
    <div class="brand"><img src="data:image/svg+xml;base64,${mark}">Agent Desk</div>
    <h1>Your Claude Code agents, on one desk.</h1>
    <p>A local GUI for agent-manager: read every session, answer its questions, review and reply, from your desk or your phone.</p>
  </div>
  <div class="cmd"><span>$</span> brew install gsperry/tap/agent-desk</div>
  <img class="shot" src="data:image/png;base64,${shot}">
</body></html>`

const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1200, height: 630 } })
await page.setContent(html)
await page.waitForTimeout(300)
await page.screenshot({ path: join(root, 'site', 'img', 'og.png') })
await browser.close()
console.log('wrote site/img/og.png')
