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

test('snippets insert from the menu and from /name plus Tab', async ({ page }) => {
  await open(page, 'a0000003')
  const box = composer(page)
  await box.fill('/tests')
  await box.press('Tab')
  await expect(box).toHaveValue('Run the full test suite and fix anything that fails before moving on.')
  await box.fill('')
  await page.getByRole('button', { name: 'Snippets' }).click()
  await page.getByRole('menuitem', { name: /\/status/ }).click()
  await expect(box).toHaveValue(/short status/)
})

test('a snippet added in Settings fills in variables', async ({ page }) => {
  await open(page, 'a0000003')
  await page.getByRole('button', { name: 'Settings' }).click()
  const dialog = page.getByRole('dialog', { name: 'Settings' })
  await dialog.getByRole('button', { name: 'Add snippet' }).click()
  await dialog.getByRole('textbox', { name: 'Snippet name' }).last().fill('where')
  await dialog.getByRole('textbox', { name: 'Snippet text' }).last().fill('You are {session} in {project}.')
  await dialog.getByRole('button', { name: 'Done' }).click()
  const box = composer(page)
  await box.fill('/where')
  await box.press('Tab')
  await expect(box).toHaveValue('You are probe in probe.')
})

test('a pasted image is stored and its path inserted', async ({ page }) => {
  await open(page, 'a0000003')
  const box = composer(page)
  await box.fill('Look at this:')
  await box.evaluate((el) => {
    const bytes = Uint8Array.from(atob('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII='), (c) => c.charCodeAt(0))
    const dt = new DataTransfer()
    dt.items.add(new File([bytes], 'shot.png', { type: 'image/png' }))
    const ta = el as HTMLTextAreaElement
    ta.setSelectionRange(ta.value.length, ta.value.length)
    ta.dispatchEvent(new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true }))
  })
  await expect(box).toHaveValue(/^Look at this: \/.*\/pastes\/paste-[^/]+\.png$/)
})

test('slash autocomplete offers Claude Code commands, skills and snippets', async ({ page }) => {
  await page.goto(`${base}/s/a0000011#t=${token}`)
  const box = page.getByRole('textbox', { name: 'Reply to reviewer' })
  await box.fill('/comp')
  const menu = page.getByRole('listbox', { name: 'Commands' })
  await expect(menu.getByRole('option').first()).toContainText('/compact')
  await expect(menu.getByRole('option').first()).toContainText('Claude Code')
  await box.press('Enter')
  await expect(box).toHaveValue('/compact ')
  await expect(menu).toHaveCount(0)

  await box.fill('/tri')
  await expect(menu.getByRole('option', { name: /\/triage/ })).toContainText('skill · user')
  await box.press('Escape')
  await expect(menu).toHaveCount(0)
  await expect(box).toHaveValue('/tri')

  await box.fill('/sh')
  await expect(menu.getByRole('option').first()).toContainText('/ship')
  await expect(menu.getByRole('option').first()).toContainText('<version>')

  // Arrow keys move; Tab picks. A snippet expands to its text.
  await box.fill('/cont')
  await expect(menu.getByRole('option').first()).toContainText('/continue')
  await expect(menu.getByRole('option').first()).toContainText('snippet')
  await box.press('ArrowDown')
  await box.press('ArrowUp')
  await box.press('Tab')
  await expect(box).toHaveValue('Continue.')
})
