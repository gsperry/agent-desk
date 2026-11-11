import { expect, test } from '@playwright/test'
import { port, token } from '../playwright.config'

const base = `http://127.0.0.1:${String(port)}`

test('changes show the diff, and line comments go out as one review', async ({ page }) => {
  await page.goto(`${base}/s/a0000011#t=${token}`)
  await page.getByRole('button', { name: 'Changes', exact: true }).click()
  const panel = page.getByRole('complementary', { name: 'Changes' })
  await expect(panel.locator('.changed-file')).toContainText('notes.md')
  await expect(panel.locator('.changed-file')).toContainText('+2')
  await panel.locator('tr.diff-add').filter({ hasText: 'TWO' }).click()
  await panel.getByRole('textbox', { name: 'Review comment' }).fill('Keep this lowercase.')
  await panel.getByRole('button', { name: 'Add comment' }).click()
  await expect(panel.locator('.review-comment')).toContainText('Keep this lowercase.')
  await panel.getByRole('button', { name: 'Send review' }).click()
  await expect(panel).toBeHidden()
  const main = page.getByRole('main')
  await expect(main.locator('.card.user').filter({ hasText: 'Review comments on your changes' })).toContainText('1. notes.md:2')
  await expect(main.locator('.pending')).toHaveCount(0, { timeout: 10_000 })
})

test('a session outside a repository says so', async ({ page }) => {
  await page.goto(`${base}/s/a0000009#t=${token}`)
  await page.getByRole('button', { name: 'Changes', exact: true }).click()
  await expect(page.getByRole('complementary', { name: 'Changes' })).toContainText('not inside a git repository')
})

test('the activity filter narrows the transcript and risky commands stand out', async ({ page }) => {
  await page.goto(`${base}/s/a0000009#t=${token}`)
  const main = page.getByRole('main')
  await expect(main.locator('.tool')).toHaveCount(2)
  await main.getByRole('radiogroup', { name: 'Show' }).getByRole('radio', { name: 'Commands' }).click()
  await expect(main.locator('.tool')).toHaveCount(1)
  await expect(main.locator('.tool')).toContainText('Bash')
  await main.getByRole('radiogroup', { name: 'Show' }).getByRole('radio', { name: 'File changes' }).click()
  await expect(main.locator('.tool')).toHaveCount(0)
  await expect(main.getByText('Nothing of this kind')).toBeVisible()
})
