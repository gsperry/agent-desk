import { readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { expect, test } from '@playwright/test'
import { port, token } from '../playwright.config'

const base = `http://127.0.0.1:${String(port)}`
const snapshotPath = join(import.meta.dirname, '..', '.e2e-fixtures', 'snapshot.json')
let original = ''

// Resuming spawns a fixture session; put the snapshot back afterwards.
test.beforeEach(() => {
  original = readFileSync(snapshotPath, 'utf8')
})

test.afterEach(() => {
  writeFileSync(snapshotPath, original)
})
const past = '0a1b2c3d-0000-4000-8000-000000000a1b'

test('all conversations lists transcripts on disk and filters them', async ({ page }) => {
  await page.goto(`${base}/#t=${token}`)
  await page.getByRole('button', { name: 'All conversations' }).click()
  await expect(page).toHaveURL(/\/history$/)
  const main = page.getByRole('main', { name: 'All conversations' })
  await expect(main.locator('.conversation-row').filter({ hasText: 'Settings table migration plan' })).toContainText('tmp')
  await expect(main.locator('.conversation-row').filter({ hasText: 'session orchestrator' })).toHaveCount(1)
  await main.getByRole('searchbox').fill('migration')
  await expect(main.locator('.conversation-row')).toHaveCount(1)
  await main.locator('.conversation-row').click()
  await expect(page).toHaveURL(new RegExp(`/history/${past}$`))
  const view = page.getByRole('main')
  await expect(view.getByRole('heading', { name: 'Settings table migration plan' })).toBeVisible()
  await expect(view).toContainText('Wrote MIGRATION.md')
  await view.getByRole('button', { name: '← All conversations' }).click()
  await expect(page).toHaveURL(/\/history$/)
})

test('a past conversation resumes as a new agent-manager session', async ({ page }) => {
  await page.goto(`${base}/history/${past}#t=${token}`)
  await page.getByRole('button', { name: 'Resume in agent-manager' }).click()
  const dialog = page.getByRole('dialog', { name: 'Resume conversation' })
  await expect(dialog).toContainText('Settings table migration plan')
  await expect(dialog.getByLabel('Directory')).toHaveValue('/tmp')
  await expect(dialog.getByLabel('Directory')).not.toBeEditable()
  await expect(dialog.getByLabel('CLI')).toHaveCount(0)
  await expect(dialog.getByText('Own git worktree')).toHaveCount(0)
  await dialog.getByLabel('Name').fill('migration-resumed')
  await dialog.getByRole('button', { name: 'Resume' }).click()
  await expect(page).toHaveURL(/\/s\/[0-9a-f]+$/)
  await expect(page.getByRole('main').getByRole('heading', { name: 'migration-resumed' })).toBeVisible()
  await expect(page.getByRole('main')).toContainText('Wrote MIGRATION.md')
})

test('a conversation a session runs links to the session instead', async ({ page }) => {
  await page.goto(`${base}/history/cccccccc-cccc-4ccc-8ccc-cccccccccccc#t=${token}`)
  await expect(page.getByRole('button', { name: 'Resume in agent-manager' })).toHaveCount(0)
  await page.getByRole('button', { name: 'Open session orchestrator' }).click()
  await expect(page).toHaveURL(/\/s\/a0000010$/)
})

test('a conversation exports to Markdown', async ({ page }) => {
  await page.goto(`${base}/history/${past}#t=${token}`)
  const download = page.waitForEvent('download')
  await page.getByRole('button', { name: 'Export' }).click()
  const file = await download
  expect(file.suggestedFilename()).toBe('settings-table-migration-plan.md')
  const path = await file.path()
  const text = readFileSync(path, 'utf8')
  expect(text).toContain('# Settings table migration plan')
  expect(text).toContain('## You · 2026-11-01 09:00 UTC')
  expect(text).toContain('Wrote MIGRATION.md')
})

test('a session exports its conversation', async ({ page }) => {
  await page.goto(`${base}/s/a0000011#t=${token}`)
  const download = page.waitForEvent('download')
  await page.getByRole('main').getByRole('button', { name: 'Session actions' }).click()
  await page.getByRole('main').getByRole('menuitem', { name: 'Export' }).click()
  expect((await download).suggestedFilename()).toMatch(/\.md$/)
})

test('a session template fills the new session form', async ({ page }) => {
  await page.goto(`${base}/#t=${token}`)
  const nav = page.getByRole('navigation', { name: 'Sessions' })
  await nav.getByRole('button', { name: 'New session', exact: true }).click()
  let dialog = page.getByRole('dialog', { name: 'New session' })
  await expect(dialog.getByLabel('Template')).toHaveCount(0)
  await dialog.getByPlaceholder('/path/to/project').fill('/tmp')
  await dialog.getByLabel('First prompt').fill('Run the tests and report.')
  await dialog.getByRole('button', { name: 'Save as template' }).click()
  await dialog.getByLabel('Template name').fill('test-run')
  await dialog.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(dialog.getByRole('button', { name: 'Saved “test-run”' })).toBeVisible()
  await dialog.getByRole('button', { name: 'Cancel' }).click()

  await nav.getByRole('button', { name: 'New session', exact: true }).click()
  dialog = page.getByRole('dialog', { name: 'New session' })
  await expect(dialog.getByPlaceholder('/path/to/project')).toHaveValue('')
  await dialog.getByLabel('Template').selectOption('test-run')
  await expect(dialog.getByPlaceholder('/path/to/project')).toHaveValue('/tmp')
  await expect(dialog.getByLabel('First prompt')).toHaveValue('Run the tests and report.')
  await dialog.getByRole('button', { name: 'Cancel' }).click()

  await nav.getByRole('button', { name: 'Settings' }).click()
  const settings = page.getByRole('dialog', { name: 'Settings' })
  await expect(settings.locator('.template-list')).toContainText('test-run')
  await settings.getByRole('button', { name: 'Remove template test-run' }).click()
  await expect(settings).toContainText('No templates yet.')
})

test('pins and a note stay with the conversation', async ({ page }) => {
  await page.goto(`${base}/s/a0000011#t=${token}`)
  const main = page.getByRole('main')
  const prompt = main.locator('.card.user').filter({ hasText: 'Update the notes.' })
  await prompt.hover()
  await prompt.getByRole('button', { name: 'Pin' }).click()
  await expect(main.getByRole('button', { name: 'Notes · 1' })).toBeVisible()
  await main.getByRole('button', { name: 'Notes · 1' }).click()
  const panel = page.getByRole('complementary', { name: 'Notes' })
  await expect(panel.locator('.pin-row')).toContainText('Update the notes.')
  await panel.getByRole('textbox', { name: 'Note', exact: true }).fill('Check the casing review.')

  await page.reload()
  await page.getByRole('main').getByRole('button', { name: 'Notes · 1' }).click()
  await expect(page.getByRole('complementary', { name: 'Notes' }).getByRole('textbox', { name: 'Note', exact: true })).toHaveValue('Check the casing review.')
  await expect(page.getByRole('main').locator('.card.user').filter({ hasText: 'Update the notes.' }).getByRole('button', { name: 'Unpin' })).toBeVisible()

  // The same conversation read from history shares the notebook.
  await page.goto(`${base}/history/dddddddd-dddd-4ddd-8ddd-dddddddddddd#t=${token}`)
  await page.getByRole('main').getByRole('button', { name: 'Notes · 1' }).click()
  const again = page.getByRole('complementary', { name: 'Notes' })
  await again.getByRole('button', { name: 'Unpin Update the notes.' }).click()
  await again.getByRole('textbox', { name: 'Note', exact: true }).fill('')
  await expect(page.getByRole('main').getByRole('button', { name: 'Notes', exact: true })).toBeVisible()
})

test('usage shows totals, a daily chart with a table view, and switches range', async ({ page }) => {
  await page.goto(`${base}/#t=${token}`)
  await page.getByRole('button', { name: 'Usage' }).click()
  await expect(page).toHaveURL(/\/usage$/)
  const main = page.getByRole('main', { name: 'Usage' })
  await expect(main.locator('.stat-tile')).toHaveCount(5)
  await expect(main.locator('.stat-tile').filter({ hasText: 'Reported cost' })).toContainText('List price')
  await expect(main.locator('.day-col')).toHaveCount(30)
  await main.getByRole('radiogroup', { name: 'Range' }).getByRole('radio', { name: '7 days' }).click()
  await expect(main.locator('.day-col')).toHaveCount(7)
  await main.getByRole('button', { name: 'Show table' }).click()
  await expect(main.locator('.usage-section').first().locator('tbody tr')).toHaveCount(7)
  await expect(main.getByRole('heading', { name: 'Models' })).toBeVisible()
})

test('catch-up summaries are off by default, and say what they cost when on', async ({ page }) => {
  await page.goto(`${base}/s/a0000011#t=${token}`)
  const main = page.getByRole('main')
  await expect(main.getByRole('heading', { name: 'reviewer' })).toBeVisible()
  await expect(main.getByRole('button', { name: 'Catch me up' })).toHaveCount(0)
  await page.getByRole('navigation', { name: 'Sessions' }).getByRole('button', { name: 'Settings' }).click()
  const settings = page.getByRole('dialog', { name: 'Settings' })
  await expect(settings).toContainText('Each summary costs tokens on your Claude Code provider.')
  await settings.getByLabel('Allow catch-up summaries').check()
  await page.keyboard.press('Escape')
  await main.getByRole('button', { name: 'Catch me up' }).click()
  const panel = main.getByRole('region', { name: 'Catch-up summary' })
  await expect(panel).toContainText('No model was called.')
  await expect(panel).toContainText('haiku · 1,500 in, 30 out tokens · $0.0012')
  await panel.getByRole('button', { name: 'Close' }).click()
  await expect(panel).toHaveCount(0)
})

test('a session opened again returns to where the reader left it', async ({ page }) => {
  await page.goto(`${base}/s/a0000002#t=${token}`)
  const nav = page.getByRole('navigation', { name: 'Sessions' })
  const scroller = page.locator('.transcript').first()
  await expect(page.getByRole('main').locator('.turn').first()).toBeVisible()
  // Read somewhere in the middle.
  await scroller.evaluate((el) => {
    el.scrollTop = Math.floor(el.scrollHeight / 3)
    el.dispatchEvent(new Event('scroll'))
  })
  await page.waitForTimeout(200)
  const before = await scroller.evaluate((el) => el.scrollTop)
  await expect(page.getByRole('button', { name: /Jump to latest/ })).toBeVisible()
  // Unblock something else, then come back.
  await nav.getByRole('button', { name: /^reviewer/ }).click()
  await expect(page.getByRole('main').getByRole('heading', { name: 'reviewer' })).toBeVisible()
  await nav.getByRole('button', { name: /^web-compaction/ }).click()
  await expect(page.getByRole('main').getByRole('heading', { name: 'web-compaction' })).toBeVisible()
  await expect.poll(() => scroller.evaluate((el) => el.scrollTop)).toBeGreaterThan(before - 40)
  expect(await scroller.evaluate((el) => el.scrollTop)).toBeLessThan(before + 40)

  // Following the latest output stays that way.
  await page.getByRole('button', { name: /Jump to latest/ }).click()
  await page.waitForTimeout(200)
  await nav.getByRole('button', { name: /^reviewer/ }).click()
  await nav.getByRole('button', { name: /^web-compaction/ }).click()
  await expect.poll(() => scroller.evaluate((el) => el.scrollHeight - el.scrollTop - el.clientHeight)).toBeLessThan(80)
})
