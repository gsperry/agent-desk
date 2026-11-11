import { existsSync, readFileSync, rmSync } from 'node:fs'
import { join } from 'node:path'
import { expect, test, type Page } from '@playwright/test'
import { port, token } from '../playwright.config'

const base = `http://127.0.0.1:${String(port)}`
const openLog = join(import.meta.dirname, '..', '.e2e-fixtures', 'open.log')

async function open(page: Page, sessionId: string) {
  await page.goto(`${base}/s/${sessionId}#t=${token}`)
}

function opened(): string {
  return existsSync(openLog) ? readFileSync(openLog, 'utf8') : ''
}

test.beforeEach(() => {
  rmSync(openLog, { force: true })
})

test('settings change the theme, font size and width, and persist', async ({ page }) => {
  await open(page, 'a0000009')
  await page.getByRole('button', { name: 'Settings' }).click()
  const dialog = page.getByRole('dialog', { name: 'Settings' })
  await dialog.getByRole('radio', { name: 'Dark' }).click()
  await dialog.getByRole('button', { name: 'Larger text' }).click()
  await dialog.getByRole('button', { name: 'Larger text' }).click()
  await dialog.getByRole('radio', { name: 'Wide' }).click()
  const root = page.locator('html')
  await expect(root).toHaveAttribute('data-theme', 'dark')
  const vars = () =>
    page.evaluate(() => {
      const s = document.documentElement.style
      return [s.getPropertyValue('--font-size'), s.getPropertyValue('--reading-width')]
    })
  expect(await vars()).toEqual(['17px', '60rem'])
  await page.reload()
  await expect(root).toHaveAttribute('data-theme', 'dark')
  expect(await vars()).toEqual(['17px', '60rem'])
})

test('collapse defaults decide how tool calls start', async ({ page }) => {
  await open(page, 'a0000009')
  const bash = page.locator('.tool').filter({ hasText: 'Say hello' })
  await expect(bash.locator('.tool-body')).toHaveCount(0)
  await page.getByRole('button', { name: 'Settings' }).click()
  await page.getByRole('radiogroup', { name: 'Shell commands' }).getByRole('radio', { name: 'Expanded' }).click()
  await page.getByRole('button', { name: 'Done' }).click()
  await page.reload()
  await expect(bash.locator('.tool-body')).toContainText('hello-from-bash')
})

test('file paths open in the editor, and a missing file says so', async ({ page }) => {
  await open(page, 'a0000009')
  await page.getByRole('button', { name: '/etc/hosts', exact: true }).click()
  await expect.poll(opened).toBe('code -g /etc/hosts\n')
  const read = page.locator('.tool').filter({ hasText: 'Read' })
  await read.hover()
  await read.getByRole('button', { name: 'Open' }).click()
  await expect.poll(opened).toBe('code -g /etc/hosts\ncode -g /etc/hosts:3\n')
  await page.getByRole('button', { name: '/nonexistent/missing.go', exact: true }).click()
  await expect(page.getByRole('status')).toContainText('does not exist')
})

test('search finds a phrase, shows it in context and opens the session', async ({ page }) => {
  await open(page, 'a0000001')
  await expect(page.getByRole('navigation', { name: 'Sessions' }).getByText('files-demo')).toBeVisible()
  await page.keyboard.press('/')
  await expect(page.getByRole('searchbox', { name: 'Search transcripts' })).toBeFocused()
  await page.keyboard.type('unique-search-phrase')
  const results = page.getByRole('main', { name: 'Search results' })
  const hit = results.locator('.hit').filter({ hasText: 'files-demo' })
  await expect(hit).toHaveCount(1)
  await expect(hit.locator('mark')).toHaveText('unique-search-phrase')
  await hit.locator('.hit-row').click()
  await expect(hit.locator('.hit-target')).toContainText('unique-search-phrase')
  await hit.getByRole('button', { name: 'Open session' }).click()
  await expect(page).toHaveURL(/\/s\/a0000009$/)
  await expect(results).toBeHidden()
  await expect(page.getByRole('searchbox', { name: 'Search transcripts' })).toHaveValue('')
})

test('Escape clears the search', async ({ page }) => {
  await open(page, 'a0000001')
  const box = page.getByRole('searchbox', { name: 'Search transcripts' })
  await box.fill('unique-search-phrase')
  await expect(page.getByRole('main', { name: 'Search results' })).toBeVisible()
  await box.press('Escape')
  await expect(page.getByRole('main', { name: 'Search results' })).toBeHidden()
})
