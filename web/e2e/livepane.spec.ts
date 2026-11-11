import { existsSync, readFileSync, rmSync } from 'node:fs'
import { join } from 'node:path'
import { expect, test, type Page } from '@playwright/test'
import { port, token } from '../playwright.config'

const base = `http://127.0.0.1:${String(port)}`
const keysLog = join(import.meta.dirname, '..', '.e2e-fixtures', 'keys.log')

async function open(page: Page, sessionId: string) {
  await page.goto(`${base}/s/${sessionId}#t=${token}`)
}

function keys(): string {
  return existsSync(keysLog) ? readFileSync(keysLog, 'utf8') : ''
}

test.beforeEach(() => {
  rmSync(keysLog, { force: true })
})

test('a waiting session shows its prompt with a button per option', async ({ page }) => {
  await open(page, 'a0000002')
  const pane = page.getByRole('region', { name: 'Terminal' })
  await expect(pane.getByText('Do you want to proceed?').first()).toBeVisible()
  const options = pane.locator('.live-option')
  await expect(options).toHaveCount(4)
  await expect(options.first()).toHaveClass(/selected/)
  await options.filter({ hasText: /^4 No$/ }).click()
  await expect.poll(keys).toBe('a0000002 Down Down Down Enter\n')
})

test('Cancel presses Escape', async ({ page }) => {
  await open(page, 'a0000002')
  await page.getByRole('button', { name: 'Cancel (Esc)' }).click()
  await expect.poll(keys).toBe('a0000002 Escape\n')
})

test('the Terminal button shows any running session and Hide closes it', async ({ page }) => {
  await open(page, 'a0000001')
  const pane = page.getByRole('region', { name: 'Terminal' })
  await expect(pane).toBeHidden()
  await page.getByRole('button', { name: 'Terminal' }).click()
  await expect(pane.locator('.live-screen')).toContainText('❯')
  await pane.getByRole('button', { name: 'Hide' }).click()
  await expect(pane).toBeHidden()
})

test('a reply refused for a dialog opens the terminal', async ({ page }) => {
  await open(page, 'a0000008')
  await page.getByRole('textbox', { name: /Reply to/ }).fill('anything')
  await page.getByRole('button', { name: 'Send', exact: true }).click()
  const pane = page.getByRole('region', { name: 'Terminal' })
  await expect(pane.locator('.live-option')).toHaveCount(2)
})
