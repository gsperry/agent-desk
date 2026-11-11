import { expect, test } from '@playwright/test'
import { port, token } from '../playwright.config'

const base = `http://127.0.0.1:${String(port)}`

test.use({ viewport: { width: 390, height: 844 } })

test('on a phone the session list and a session take turns, full screen', async ({ page }) => {
  await page.goto(`${base}/#t=${token}`)
  const nav = page.getByRole('navigation', { name: 'Sessions' })
  await expect(nav).toBeVisible()
  await expect(page.locator('.main-col')).toBeHidden()

  await nav.getByRole('button', { name: /^reviewer/ }).click()
  await expect(nav).toBeHidden()
  const main = page.getByRole('main')
  await expect(main.getByRole('heading', { name: 'reviewer' })).toBeVisible()
  await expect(page.getByRole('textbox', { name: 'Reply to reviewer' })).toBeVisible()
  // Nothing is wider than the screen.
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
  expect(overflow).toBeLessThanOrEqual(1)

  await page.getByRole('button', { name: '‹ Sessions' }).click()
  await expect(nav).toBeVisible()
  await expect(page).toHaveURL(`${base}/`)
})

test('on a phone a waiting session can be answered without scrolling sideways', async ({ page }) => {
  await page.goto(`${base}/s/a0000002#t=${token}`)
  const pane = page.getByRole('region', { name: 'Terminal' })
  const yes = pane.getByRole('button', { name: /^1 Yes/ })
  await expect(yes).toBeVisible()
  const box = await yes.boundingBox()
  expect(box?.height ?? 0).toBeGreaterThanOrEqual(44)
  expect((box?.x ?? 0) + (box?.width ?? 0)).toBeLessThanOrEqual(390)
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
  expect(overflow).toBeLessThanOrEqual(1)
})
