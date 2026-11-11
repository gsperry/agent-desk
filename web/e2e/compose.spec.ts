import { expect, test, type Page } from '@playwright/test'
import { port, token } from '../playwright.config'

const base = `http://127.0.0.1:${String(port)}`

async function open(page: Page, sessionId: string) {
  await page.goto(`${base}/s/${sessionId}#t=${token}`)
}

function composer(page: Page) {
  return page.getByRole('textbox', { name: /Reply to/ })
}

test('Enter adds a line and the send key sends, then the transcript confirms it', async ({ page }) => {
  await open(page, 'a0000003')
  const box = composer(page)
  await box.click()
  await box.pressSequentially('Hello from the smoke test')
  await box.press('Enter')
  await box.pressSequentially('second line')
  await expect(box).toHaveValue('Hello from the smoke test\nsecond line')
  await box.press('ControlOrMeta+Enter')
  await expect(box).toHaveValue('')
  const main = page.getByRole('main')
  await expect(main.locator('.card.user').filter({ hasText: 'second line' })).toBeVisible()
  // Reconciled: the fixture recorded the prompt and the pending bubble is gone.
  await expect(main.locator('.pending')).toHaveCount(0, { timeout: 10_000 })
  await expect(main.locator('.card.user').filter({ hasText: 'second line' })).toHaveCount(1)
})

test('drafts are kept per session', async ({ page }) => {
  await open(page, 'a0000003')
  await composer(page).fill('a draft for the probe')
  await page.getByRole('navigation', { name: 'Sessions' }).getByText('api-orchestrator').click()
  await expect(composer(page)).toHaveValue('')
  await page.getByRole('navigation', { name: 'Sessions' }).locator('.row-name', { hasText: /^probe$/ }).click()
  await expect(composer(page)).toHaveValue('a draft for the probe')
  await page.reload()
  await expect(composer(page)).toHaveValue('a draft for the probe')
})

test('a waiting session explains why it cannot take a reply', async ({ page }) => {
  await open(page, 'a0000002')
  await expect(page.getByText('waiting on a question or permission prompt')).toBeVisible()
  await composer(page).fill('anything')
  await expect(page.getByRole('button', { name: 'Send', exact: true })).toBeDisabled()
})

test('a dialog on screen refuses the send and the text can be edited', async ({ page }) => {
  await open(page, 'a0000008')
  await composer(page).fill('pick nothing')
  await page.getByRole('button', { name: 'Send', exact: true }).click()
  const failed = page.locator('.pending-failed')
  await expect(failed).toContainText('showing a dialog')
  await failed.getByRole('button', { name: 'Edit' }).click()
  await expect(failed).toHaveCount(0)
  await expect(composer(page)).toHaveValue('pick nothing')
})
