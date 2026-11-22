import { rmSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { expect, test } from '@playwright/test'
import { port, token } from '../playwright.config'

const base = `http://127.0.0.1:${String(port)}`
// In fixture mode read aloud is a stand-in that answers with silence and
// makes no network call; this file turns it on.
const marker = join(import.meta.dirname, '..', '.e2e-fixtures', 'tts.json')

test.afterEach(() => {
  rmSync(marker, { force: true })
})

test('without a text-to-speech setup there is no Read aloud button', async ({ page }) => {
  await page.goto(`${base}/s/a0000001#t=${token}`)
  await expect(page.locator('.pinnable').first()).toBeVisible()
  await expect(page.getByRole('button', { name: 'Read aloud' })).toHaveCount(0)
  await page.getByRole('button', { name: 'Settings' }).click()
  await expect(page.getByRole('dialog', { name: 'Settings' })).toContainText('put your API key in tts.json')
})

test('with one, a reply reads aloud and the button resets when it ends', async ({ page }) => {
  writeFileSync(marker, '{}')
  await page.goto(`${base}/s/a0000001#t=${token}`)
  const button = page.locator('.pinnable').first().getByRole('button', { name: 'Read aloud' })
  await expect(button).toHaveCount(1)
  await button.click()
  // The audio is a short silence: the button goes busy, then back.
  await expect(page.locator('.speak-button[aria-pressed="true"]')).toHaveCount(1)
  await expect(page.locator('.speak-button[aria-pressed="true"]')).toHaveCount(0, { timeout: 10_000 })
  await expect(page.locator('.speak-error')).toHaveCount(0)
  await page.getByRole('button', { name: 'Settings' }).click()
  await expect(page.getByRole('dialog', { name: 'Settings' })).toContainText('reads them with fixture')
})
