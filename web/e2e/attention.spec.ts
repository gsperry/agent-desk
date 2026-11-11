import { readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { expect, test, type Page } from '@playwright/test'
import { port, token } from '../playwright.config'

const base = `http://127.0.0.1:${String(port)}`
const snapshotPath = join(import.meta.dirname, '..', '.e2e-fixtures', 'snapshot.json')

async function open(page: Page, sessionId: string) {
  await page.goto(`${base}/s/${sessionId}#t=${token}`)
}

const nav = (page: Page) => page.getByRole('navigation', { name: 'Sessions' })

test('the badge and title count sessions that need you, and viewing a finished one clears it', async ({ page }) => {
  await open(page, 'a0000001')
  // waiting web-compaction, unseen finished probe and dialog-demo
  await expect(page).toHaveTitle('(3) Agent Desk')
  await expect(nav(page).getByRole('button', { name: /Needs you/ })).toContainText('3')
  await nav(page).locator('.row-name', { hasText: /^probe$/ }).click()
  await expect(page).toHaveTitle('(2) Agent Desk')
})

test('the filter shows only sessions that need you', async ({ page }) => {
  await open(page, 'a0000001')
  await nav(page).getByRole('button', { name: /Needs you/ }).click()
  await expect(nav(page).locator('.row')).toHaveCount(3)
  await expect(nav(page).getByText('api-orchestrator')).toBeHidden()
})

test('j and k step through sessions that need you', async ({ page }) => {
  await open(page, 'a0000001')
  await expect(page).toHaveTitle('(3) Agent Desk')
  await page.keyboard.press('j')
  await expect(page).toHaveURL(/\/s\/a0000008$/)
  await page.keyboard.press('j')
  await expect(page).toHaveURL(/\/s\/a0000003$/)
  await page.keyboard.press('k')
  await expect(page).toHaveURL(/\/s\/a0000002$/)
})

test('j does nothing while typing in the composer', async ({ page }) => {
  await open(page, 'a0000001')
  await page.getByRole('textbox', { name: /Reply to/ }).click()
  await page.keyboard.press('j')
  await expect(page).toHaveURL(/\/s\/a0000001$/)
  await expect(page.getByRole('textbox', { name: /Reply to/ })).toHaveValue('j')
  await page.keyboard.press('Escape')
  await page.keyboard.press('j')
  await expect(page).toHaveURL(/\/s\/a0000008$/)
})

test('Cmd+K switches sessions by name', async ({ page }) => {
  await open(page, 'a0000001')
  await expect(nav(page).getByText('api-orchestrator')).toBeVisible()
  await page.keyboard.press('ControlOrMeta+k')
  const dialog = page.getByRole('dialog', { name: 'Switch session' })
  await expect(dialog).toBeVisible()
  await page.keyboard.type('codex')
  await page.keyboard.press('Enter')
  await expect(dialog).toBeHidden()
  await expect(page).toHaveURL(/\/s\/a0000006$/)
})

test('a session flipping to waiting raises a notification', async ({ page }) => {
  await page.addInitScript(() => {
    const notes: string[] = []
    Object.assign(window, { __notes: notes })
    class FakeNotification {
      static permission = 'granted'
      static requestPermission() {
        return Promise.resolve('granted')
      }
      onclick: (() => void) | null = null
      constructor(title: string, opts?: { body?: string }) {
        notes.push(`${title} | ${opts?.body ?? ''}`)
      }
      close() {
        return undefined
      }
    }
    Object.assign(window, { Notification: FakeNotification })
  })
  const original = readFileSync(snapshotPath, 'utf8')
  try {
    await open(page, 'a0000001')
    await expect(page).toHaveTitle('(3) Agent Desk')
    const snap = JSON.parse(original) as { sessions: { id: string; status: string }[] }
    const fresh = snap.sessions.find((s) => s.id === 'a0000005')
    if (fresh) fresh.status = 'waiting'
    writeFileSync(snapshotPath, JSON.stringify(snap))
    await expect
      .poll(() => page.evaluate(() => (window as unknown as { __notes: string[] }).__notes), { timeout: 5000 })
      .toContain('◆ fresh-start is waiting for you | lab')
    await expect(page).toHaveTitle('(4) Agent Desk')
  } finally {
    writeFileSync(snapshotPath, original)
  }
})
