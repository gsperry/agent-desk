import { expect, test, type Page } from '@playwright/test'
import { port, token } from '../playwright.config'

const base = `http://127.0.0.1:${String(port)}`
const conv = 'cccccccc-cccc-4ccc-8ccc-cccccccccccc'

async function open(page: Page, path: string) {
  await page.goto(`${base}${path}#t=${token}`)
}

test('the overview is the home screen and opens a session', async ({ page }) => {
  await open(page, '/')
  const overview = page.getByRole('main', { name: 'Overview' })
  const card = overview.locator('.session-card').filter({ has: page.locator('.card-name', { hasText: /^orchestrator$/ }) })
  await expect(card).toContainText('Agent: Audit row 1')
  await expect(card).toContainText('You: Run the release initiative.')
  // Needs-you sessions come first.
  await expect(overview.locator('.session-card').first().locator('.status-chip')).toHaveText(/waiting|finished|errored/)
  await card.click()
  await expect(page).toHaveURL(/\/s\/a0000010$/)
  await page.getByRole('button', { name: 'Agent Desk', exact: true }).click()
  await expect(overview).toBeVisible()
})

test('the context meter learns the compaction point', async ({ page }) => {
  await open(page, '/s/a0000010')
  const meter = page.getByLabel('Context used')
  await expect(meter).toContainText('Context 400k')
  await expect(meter).toContainText('50%')
})

test('subagents are listed with status and their transcripts open', async ({ page }) => {
  await open(page, '/s/a0000010')
  const toggle = page.getByRole('button', { name: /Agents 1 running · 2/ })
  await toggle.click()
  const panel = page.getByRole('complementary', { name: 'Subagents' })
  const rows = panel.locator('.agent-row')
  await expect(rows).toHaveCount(2)
  await expect(rows.first()).toContainText('auditor')
  await expect(rows.first()).toContainText('running')
  await expect(rows.nth(1)).toContainText('completed')
  await rows.nth(1).click()
  await expect(panel).toContainText('Row 1 implemented and tested.')
  await panel.getByRole('button', { name: '← All agents' }).click()
  await expect(rows).toHaveCount(2)
})

test('an Agent tool call links to its subagent', async ({ page }) => {
  await open(page, '/s/a0000010')
  await expect(page.getByRole('button', { name: /Agents/ })).toBeVisible()
  await page.locator('.tool').filter({ hasText: 'Execute row 1' }).getByRole('button', { name: 'View transcript' }).click()
  await expect(page.getByRole('complementary', { name: 'Subagents' })).toContainText('Row 1 implemented and tested.')
})

test('new activity shows a dot and a divider', async ({ page }) => {
  await page.addInitScript((c) => {
    localStorage.setItem('agent-desk:seen', JSON.stringify({ [c]: { at: '2020-01-01T00:00:00Z', seq: 3 } }))
  }, conv)
  await open(page, '/s/a0000001')
  const row = page.getByRole('navigation', { name: 'Sessions' }).locator('.row').filter({ has: page.locator('.row-name', { hasText: /^orchestrator$/ }) })
  await expect(row.locator('.unread')).toBeVisible()
  await row.click()
  await expect(page.getByRole('separator').filter({ hasText: 'New since you last looked' })).toBeVisible()
  await expect(row.locator('.unread')).toHaveCount(0)
})
