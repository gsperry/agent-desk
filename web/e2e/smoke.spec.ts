import { expect, test, type Page } from '@playwright/test'
import { port, token } from '../playwright.config'

const base = `http://127.0.0.1:${String(port)}`

async function open(page: Page, sessionId: string) {
  await page.goto(`${base}/s/${sessionId}#t=${token}`)
}

test('sidebar groups sessions by project with status', async ({ page }) => {
  await open(page, 'a0000003')
  const nav = page.getByRole('navigation', { name: 'Sessions' })
  await expect(nav.getByText('api-orchestrator')).toBeVisible()
  await expect(nav.getByText('web-compaction')).toBeVisible()
  await expect(nav.getByRole('heading', { name: 'lab' })).toBeVisible()
  await expect(nav.getByText('old-spike')).toBeHidden()
  await nav.getByText('Show archived (1)').click()
  await expect(nav.getByText('old-spike')).toBeVisible()
  expect(page.url()).not.toContain('#t=')
})

test('transcript renders prompts, replies and tool calls', async ({ page }) => {
  await open(page, 'a0000003')
  const main = page.getByRole('main')
  await expect(main.getByRole('heading', { name: 'probe' })).toBeVisible()
  await expect(main.getByText('You').first()).toBeVisible()
  await expect(main.locator('.tool').first()).toBeVisible()
  await expect(main.getByText('agent-manager note').first()).toBeVisible()
})

test('session view screenshot', async ({ page }) => {
  await open(page, 'a0000001')
  const main = page.getByRole('main')
  await expect(main.locator('.tool').first()).toBeVisible()
  // Let highlighting settle before the screenshot.
  await page.waitForTimeout(500)
  await page.screenshot({ path: process.env.SCREENSHOT_PATH ?? 'test-results/session.png' })
})

test.describe('failure states are explicit', () => {
  const cases = [
    ['a0000005', 'Waiting for the first turn'],
    ['a0000006', 'This session runs codex'],
    ['zzzzzzzz', 'no longer exists'],
  ] as const
  for (const [id, text] of cases) {
    test(id, async ({ page }) => {
      await open(page, id)
      await expect(page.getByRole('main').getByText(text)).toBeVisible()
    })
  }
})

test('unknown records show as raw events', async ({ page }) => {
  await open(page, 'a0000004')
  await expect(page.getByRole('main').getByText('Unrecognized event').first()).toBeVisible()
})

test('a dead session says so', async ({ page }) => {
  await open(page, 'a0000007')
  await expect(page.getByRole('main').getByText('This session is not running')).toBeVisible()
})
