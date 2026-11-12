import { readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { expect, test, type Page } from '@playwright/test'
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

async function open(page: Page, sessionId: string) {
  await page.goto(`${base}/s/${sessionId}#t=${token}`)
  await expect(page.getByRole('navigation', { name: 'Sessions' }).getByText('files-demo')).toBeVisible()
}

const nav = (page: Page) => page.getByRole('navigation', { name: 'Sessions' })

test('a new session starts from the form and opens', async ({ page }) => {
  await open(page, 'a0000001')
  await nav(page).getByRole('button', { name: 'New session', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'New session' })
  await expect(dialog.getByLabel('CLI')).toHaveValue('claude')
  await dialog.getByPlaceholder('/path/to/project').fill('relative/path')
  await expect(dialog.getByText('Use a full path')).toBeVisible()
  await expect(dialog.getByRole('button', { name: 'Start' })).toBeDisabled()
  await dialog.getByPlaceholder('/path/to/project').fill('/tmp')
  await dialog.getByPlaceholder('Left empty, the agent names it').fill('e2e-new')
  await expect(dialog.getByRole('button', { name: 'Start' })).toBeEnabled()
  // The send key in the first prompt starts it, as Start does.
  await dialog.getByLabel('First prompt').fill('List the files.')
  await dialog.getByLabel('First prompt').press('ControlOrMeta+Enter')
  await expect(dialog).toBeHidden()
  await expect(page.getByRole('main').getByRole('heading', { name: 'e2e-new' })).toBeVisible()
  await expect(nav(page).getByText('e2e-new')).toBeVisible()
})

test('the + on a group prefills it', async ({ page }) => {
  await open(page, 'a0000001')
  await nav(page).getByRole('heading', { name: 'lab' }).hover()
  await nav(page).getByRole('button', { name: 'New session in lab' }).click()
  const dialog = page.getByRole('dialog', { name: 'New session' })
  await expect(dialog.getByLabel('Group')).toHaveValue('lab')
})

test('stop asks first, then revive, archive and restore', async ({ page }) => {
  await open(page, 'a0000009')
  const main = page.getByRole('main')
  await main.getByRole('button', { name: 'Stop', exact: true }).click()
  const confirm = page.getByRole('dialog', { name: 'Stop files-demo?' })
  await confirm.getByRole('button', { name: 'Stop session' }).click()
  await expect(confirm).toBeHidden()
  await expect(main.locator('.status-chip')).toHaveText('dead')
  await main.getByRole('button', { name: 'Revive' }).click()
  await expect(main.locator('.status-chip')).toHaveText('idle')
  await main.getByRole('button', { name: 'Archive' }).click()
  await expect(main.getByRole('button', { name: 'Restore' })).toBeVisible()
  await expect(nav(page).getByText('files-demo')).toBeHidden()
  await main.getByRole('button', { name: 'Restore' }).click()
  await expect(nav(page).getByText('files-demo')).toBeVisible()
})

test('a session moves to a group', async ({ page }) => {
  await open(page, 'a0000009')
  await page.getByRole('combobox', { name: 'Move to group' }).selectOption('lab')
  const lab = nav(page).locator('.group').filter({ has: page.getByRole('heading', { name: 'lab' }) })
  await expect(lab.getByText('files-demo')).toBeVisible()
})

test('groups are created and deleted', async ({ page }) => {
  await open(page, 'a0000001')
  await nav(page).getByRole('button', { name: 'New group' }).click()
  const dialog = page.getByRole('dialog', { name: 'New group' })
  await dialog.getByPlaceholder('backend/api, with / for nesting').fill('e2e-group')
  await dialog.getByRole('button', { name: 'Create' }).click()
  await expect(dialog).toBeHidden()
  const heading = nav(page).getByRole('heading', { name: 'e2e-group' })
  await expect(heading).toBeVisible()
  await heading.hover()
  await nav(page).getByRole('button', { name: 'Delete group e2e-group' }).click()
  await page.getByRole('dialog', { name: 'Delete group e2e-group?' }).getByRole('button', { name: 'Delete group' }).click()
  await expect(heading).toBeHidden()
})
