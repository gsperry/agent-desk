import { readFileSync, rmSync } from 'node:fs'
import { join } from 'node:path'
import { expect, test } from '@playwright/test'
import { port, token } from '../playwright.config'

const base = `http://127.0.0.1:${String(port)}`
const settingsPath = join(import.meta.dirname, '..', '.e2e-fixtures', 'settings.json')

// A first run: nothing in storage.
test.use({ storageState: { cookies: [], origins: [] } })

test.afterEach(() => {
  rmSync(settingsPath, { force: true })
})

test('the first run opens the setup checklist once, and Settings reopens it', async ({ page }) => {
  await page.goto(`${base}/#t=${token}`)
  await expect(page).toHaveURL(/\/welcome$/)
  const main = page.getByRole('main', { name: 'Welcome' })
  const programs = main.getByRole('region', { name: 'Programs' })
  await expect(programs).toContainText('agent-manager found')
  await expect(programs).toContainText('Claude Code (claude) found')
  await expect(main.getByRole('region', { name: 'agent-manager running' })).toContainText('Running')

  // The CLIs new sessions offer are saved through agent-manager's settings.
  const clis = main.getByRole('region', { name: 'CLIs for new sessions' })
  await clis.getByRole('checkbox', { name: 'opencode' }).uncheck()
  await clis.getByRole('checkbox', { name: 'codex' }).check()
  await clis.getByRole('combobox').selectOption('codex')
  await clis.getByRole('button', { name: 'Save' }).click()
  await expect(clis).toContainText('Saved to agent-manager.')
  const saved = JSON.parse(readFileSync(settingsPath, 'utf8')) as { key: string; value: string }[]
  expect(saved.find((s) => s.key === 'hidden_tools')?.value).toBe('opencode')
  expect(saved.find((s) => s.key === 'default_tool')?.value).toBe('codex')

  // A conversation started outside agent-manager can be resumed from here.
  const sessions = main.getByRole('region', { name: 'Your sessions' })
  await sessions.getByRole('listitem').filter({ hasText: 'Settings table migration plan' }).getByRole('button', { name: 'Resume…' }).click()
  await expect(page.getByRole('dialog')).toContainText('Resume conversation')
  await page.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click()

  await main.getByRole('button', { name: 'Go to the overview' }).click()
  await expect(page.getByRole('main', { name: 'Overview' })).toBeVisible()

  // It opens by itself only once.
  await page.goto(`${base}/#t=${token}`)
  await expect(page.getByRole('main', { name: 'Overview' })).toBeVisible()

  await page.getByRole('button', { name: 'Settings' }).click()
  await page.getByRole('button', { name: 'Setup checklist' }).click()
  await expect(page).toHaveURL(/\/welcome$/)
  await expect(page.getByRole('main', { name: 'Welcome' })).toBeVisible()
})
