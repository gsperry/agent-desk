import { readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { expect, test } from '@playwright/test'
import { port, token } from '../playwright.config'

const base = `http://127.0.0.1:${String(port)}`
const snapshotPath = join(import.meta.dirname, '..', '.e2e-fixtures', 'snapshot.json')
let original = ''

test.beforeEach(() => {
  original = readFileSync(snapshotPath, 'utf8')
})

test.afterEach(() => {
  writeFileSync(snapshotPath, original)
})

function setStatus(id: string, status: string) {
  const snap = JSON.parse(readFileSync(snapshotPath, 'utf8')) as { sessions: { id: string; status: string }[] }
  for (const s of snap.sessions) if (s.id === id) s.status = status
  writeFileSync(snapshotPath, JSON.stringify(snap))
}

test('a rule suggests a reply when a session finishes, and sends only on confirm', async ({ page }) => {
  await page.goto(`${base}/s/a0000011#t=${token}`)
  await page.getByRole('navigation', { name: 'Sessions' }).getByRole('button', { name: 'Settings' }).click()
  const settings = page.getByRole('dialog', { name: 'Settings' })
  await settings.getByRole('button', { name: 'Add rule' }).click()
  const rule = settings.getByRole('group', { name: 'Rule New rule' })
  await rule.getByLabel('Match').fill('review')
  await rule.getByLabel('Suggested reply').fill('Summarize what changed in {session}.')
  await rule.getByLabel('Rule name').fill('summary')
  await page.keyboard.press('Escape')
  await expect(settings).toBeHidden()

  const main = page.getByRole('main')
  await expect(main.getByRole('status', { name: 'Suggested reply' })).toHaveCount(0)
  // Wait for the restored status to arrive, so the change below is seen.
  await expect(main.locator('.session-head .status-chip')).toHaveText('idle')
  setStatus('a0000011', 'finished')
  const banner = main.getByRole('status', { name: 'Suggested reply' })
  await expect(banner).toContainText('Rule “summary” suggests replying:')
  await expect(banner).toContainText('Summarize what changed in reviewer.')
  await banner.getByRole('button', { name: 'Send' }).click()
  await expect(banner).toHaveCount(0)
  await expect(main.locator('.card.user').filter({ hasText: 'Summarize what changed in reviewer.' }).last()).toBeVisible()
})

test('a suggestion clears when the session moves on, and Edit puts it in the composer', async ({ page }) => {
  await page.goto(`${base}/s/a0000011#t=${token}`)
  await page.evaluate(() => {
    localStorage.setItem(
      'agent-desk:rules',
      JSON.stringify([{ id: 'r1', name: 'nudge', status: 'finished', match: '', notify: false, prompt: 'Keep going.', enabled: true }]),
    )
  })
  await page.reload()
  const main = page.getByRole('main')
  await expect(main.locator('.session-head .status-chip')).toHaveText('idle')
  setStatus('a0000011', 'finished')
  const banner = main.getByRole('status', { name: 'Suggested reply' })
  await expect(banner).toContainText('Keep going.')
  setStatus('a0000011', 'working')
  await expect(banner).toHaveCount(0)
  await expect(main.locator('.session-head .status-chip')).toHaveText('working')
  setStatus('a0000011', 'finished')
  await expect(banner).toContainText('Keep going.')
  await banner.getByRole('button', { name: 'Edit' }).click()
  await expect(banner).toHaveCount(0)
  await expect(main.getByRole('textbox', { name: 'Reply to reviewer' })).toHaveValue('Keep going.')
})
