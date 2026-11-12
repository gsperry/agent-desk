import { existsSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
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

test('the terminal shows the screen in color, read-only, and keeps the answer buttons', async ({ page }) => {
  await open(page, 'a0000002')
  const pane = page.getByRole('region', { name: 'Terminal' })
  await pane.getByRole('button', { name: 'Show screen' }).click()
  await pane.getByRole('button', { name: 'Color' }).click()
  const screen = pane.locator('.xterm-rows')
  await expect(screen).toContainText('Do you want to proceed?')
  await expect(pane.locator('.live-screen')).toHaveCount(0)
  await expect(pane.getByRole('button', { name: /^1 Yes/ })).toBeVisible()
  // Typing into the terminal sends nothing.
  await pane.locator('.xterm').click()
  await page.keyboard.type('x')
  await page.keyboard.press('Enter')
  expect(keys()).toBe('')
  // The choice is remembered.
  await page.reload()
  await expect(page.getByRole('region', { name: 'Terminal' }).locator('.xterm-rows')).toContainText('Do you want to proceed?')
  await page.getByRole('region', { name: 'Terminal' }).getByRole('button', { name: 'Plain text' }).click()
  await expect(page.getByRole('region', { name: 'Terminal' }).locator('.live-screen')).toContainText('Do you want to proceed?')
})

test('a "Type something" option takes a typed answer, and "Chat about this" is a button', async ({ page }) => {
  const panePath = join(import.meta.dirname, '..', '.e2e-fixtures', 'panes', 'a0000002.txt')
  const original = readFileSync(panePath, 'utf8')
  writeFileSync(panePath, readFileSync(join(import.meta.dirname, '..', '..', 'internal', 'pane', 'testdata', 'askq-free-initial.txt'), 'utf8'))
  try {
    await open(page, 'a0000002')
    const pane = page.getByRole('region', { name: 'Terminal' })
    await expect(pane.getByRole('button', { name: /^4 Chat about this/ })).toBeEnabled()
    await pane.getByRole('button', { name: /^3 Type something/ }).click()
    await pane.getByRole('textbox', { name: 'Your answer' }).fill('Green, with a hint of teal')
    await pane.getByRole('button', { name: 'Send answer' }).click()
    await expect(pane.getByRole('textbox', { name: 'Your answer' })).toHaveCount(0)
    // Two moves onto the text field, then the text through agent-manager type.
    await expect.poll(keys).toBe('a0000002 Down Down\n')
    await expect(page.getByRole('main').locator('.card.user').filter({ hasText: 'Green, with a hint of teal' })).toBeVisible()
  } finally {
    writeFileSync(panePath, original)
  }
})

test('a question shows without the raw screen until asked, and the choice is remembered', async ({ page }) => {
  await open(page, 'a0000002')
  const pane = page.getByRole('region', { name: 'Terminal' })
  await expect(pane.locator('.live-head')).toContainText('Question')
  await expect(pane.locator('.live-option')).toHaveCount(4)
  await expect(pane.locator('.live-screen')).toHaveCount(0)
  await expect(pane.getByRole('button', { name: 'Color' })).toHaveCount(0)
  await pane.getByRole('button', { name: 'Show screen' }).click()
  await expect(pane.locator('.live-screen')).toBeVisible()
  // Remembered across reloads and sessions.
  await page.reload()
  await expect(pane.locator('.live-screen')).toBeVisible()
  await pane.getByRole('button', { name: 'Hide screen' }).click()
  await expect(pane.locator('.live-screen')).toHaveCount(0)
  await expect(pane.locator('.live-option')).toHaveCount(4)
  // The Terminal button still shows the screen of any session.
  await open(page, 'a0000001')
  const main = page.getByRole('main')
  await main.getByRole('button', { name: 'Terminal' }).click()
  await expect(pane.locator('.live-screen')).toBeVisible()
  await expect(pane.getByRole('button', { name: 'Hide screen' })).toHaveCount(0)
})

test('Enter or Cmd+Enter submits a typed answer', async ({ page }) => {
  const panePath = join(import.meta.dirname, '..', '.e2e-fixtures', 'panes', 'a0000002.txt')
  const original = readFileSync(panePath, 'utf8')
  writeFileSync(panePath, readFileSync(join(import.meta.dirname, '..', '..', 'internal', 'pane', 'testdata', 'askq-free-initial.txt'), 'utf8'))
  try {
    await open(page, 'a0000002')
    const pane = page.getByRole('region', { name: 'Terminal' })
    await pane.getByRole('button', { name: /^3 Type something/ }).click()
    const box = pane.getByRole('textbox', { name: 'Your answer' })
    const oneRow = (await box.boundingBox())?.height ?? 0
    // A long answer wraps and the box grows to show all of it.
    await box.fill('Blue, '.repeat(60))
    await expect.poll(async () => (await box.boundingBox())?.height ?? 0).toBeGreaterThan(oneRow * 2)
    await box.fill('Blue')
    await box.press('ControlOrMeta+Enter')
    await expect(box).toHaveCount(0)
    await expect.poll(keys).toBe('a0000002 Down Down\n')
  } finally {
    writeFileSync(panePath, original)
  }
})

test('a question wrapped over several terminal lines is shown whole', async ({ page }) => {
  const panePath = join(import.meta.dirname, '..', '.e2e-fixtures', 'panes', 'a0000002.txt')
  const original = readFileSync(panePath, 'utf8')
  const sep = '─'.repeat(60)
  writeFileSync(
    panePath,
    [
      sep,
      ' ☐ Target',
      'Which environment should the release go to first? Staging',
      'runs the full suite, production skips it.',
      '❯ 1. Staging',
      '  2. Production',
      '  3. Type something.',
      sep,
      '  4. Chat about this',
      'Enter to select · ↑/↓ to navigate · Esc to cancel',
    ].join('\n'),
  )
  try {
    await open(page, 'a0000002')
    const pane = page.getByRole('region', { name: 'Terminal' })
    await expect(pane.locator('.live-question')).toHaveText(
      'Which environment should the release go to first? Staging runs the full suite, production skips it.',
    )
  } finally {
    writeFileSync(panePath, original)
  }
})

test('a several-answer question shows checkboxes and Next, and keys land on the right row', async ({ page }) => {
  const panePath = join(import.meta.dirname, '..', '.e2e-fixtures', 'panes', 'a0000002.txt')
  const original = readFileSync(panePath, 'utf8')
  writeFileSync(panePath, readFileSync(join(import.meta.dirname, '..', '..', 'internal', 'pane', 'testdata', 'askq-multi.txt'), 'utf8'))
  try {
    await open(page, 'a0000002')
    const pane = page.getByRole('region', { name: 'Terminal' })
    await expect(pane.locator('.live-question')).toHaveText(
      'Release window for the docs site: which checks do you want before it ships? Everything runs again after the deploy.',
    )
    await expect(pane.getByText('Tick each answer that applies, then Next.')).toBeVisible()
    await expect(pane.locator('.live-option')).toHaveCount(7)
    await expect(pane.locator('.live-next')).toHaveText(/Next/)
    await expect(pane.getByRole('button', { name: /^6\s+Chat about this/ })).toBeVisible()
    await expect(pane.locator('.live-option-desc').first()).toHaveText('Follow every link in the built site and report the broken ones.')
    // Chat about this sits past Next: six presses down.
    await pane.getByRole('button', { name: /Chat about this/ }).click()
    await expect.poll(keys).toBe('a0000002 Down Down Down Down Down Down Enter\n')
  } finally {
    writeFileSync(panePath, original)
  }
})

test('a prompt with several questions shows its steps', async ({ page }) => {
  const panePath = join(import.meta.dirname, '..', '.e2e-fixtures', 'panes', 'a0000002.txt')
  const original = readFileSync(panePath, 'utf8')
  writeFileSync(panePath, readFileSync(join(import.meta.dirname, '..', '..', 'internal', 'pane', 'testdata', 'askq-multi-ticked.txt'), 'utf8'))
  try {
    await open(page, 'a0000002')
    const steps = page.getByRole('region', { name: 'Terminal' }).getByRole('list', { name: 'Questions' })
    await expect(steps.getByRole('listitem')).toHaveText(['✓ Checks', 'Timing', 'Submit'])
    await expect(page.getByRole('region', { name: 'Terminal' }).getByRole('button', { name: /Link check/ })).toHaveAttribute('aria-pressed', 'true')
  } finally {
    writeFileSync(panePath, original)
  }
})

test('a working session shows its live activity, and Interrupt presses Escape', async ({ page }) => {
  await open(page, 'a0000001')
  const activity = page.getByRole('status', { name: 'Agent activity' })
  await expect(activity).toContainText('Working')
  await activity.getByRole('button', { name: 'Interrupt' }).click()
  await expect.poll(keys).toBe('a0000001 Escape\n')
  // An idle session has no activity row.
  await open(page, 'a0000004')
  await expect(page.getByRole('status', { name: 'Agent activity' })).toHaveCount(0)
})

test('Cancel is refused when no question is on screen', async ({ page }) => {
  const res = await page.request.post(`${base}/api/sessions/a0000004/answer`, {
    headers: { Authorization: `Bearer ${token}`, Origin: base, 'Content-Type': 'application/json' },
    data: { action: 'key', key: 'Escape' },
  })
  expect(res.status()).toBe(409)
  expect(keys()).toBe('')
})
