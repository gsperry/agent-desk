import { rmSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { expect, test } from '@playwright/test'
import { port, token } from '../playwright.config'

const base = `http://127.0.0.1:${String(port)}`
const statsPath = join(import.meta.dirname, '..', '.e2e-fixtures', 'stats.json')
const GB = 1024 ** 3

test.afterEach(() => {
  rmSync(statsPath, { force: true })
})

test('the sidebar shows system stats, flags a nearly full disk, and Settings hides them', async ({ page }) => {
  writeFileSync(
    statsPath,
    JSON.stringify({
      supported: true,
      at: '2026-11-13T00:00:00Z',
      cpu: { load: [3, 2, 1], cores: 12 },
      memory: { total: 36 * GB, available: 12 * GB, pressure: 'normal' },
      swap: { total: 2 * GB, used: 0 },
      disks: [{ mount: '/System/Volumes/Data', total: 460 * GB, free: 3 * GB }],
    }),
  )
  await page.goto(`${base}/#t=${token}`)
  const stats = page.getByRole('region', { name: 'System' })
  await expect(stats).toContainText('CPU')
  await expect(stats).toContainText('25%')
  await expect(stats).toContainText('24 GB / 36 GB')
  await expect(stats.locator('.sys-critical')).toContainText('3.0 GB free')
  await expect(stats.getByRole('img', { name: 'Critical' })).toHaveCount(1)

  await page.getByRole('button', { name: 'Settings' }).click()
  await page.getByRole('checkbox', { name: 'Show system stats in the sidebar' }).uncheck()
  await page.getByRole('dialog', { name: 'Settings' }).getByRole('button', { name: 'Done' }).click()
  await expect(stats).toHaveCount(0)
})

test('a disk turning critical notifies once', async ({ page }) => {
  await page.addInitScript(() => {
    const w = window as unknown as { notes: string[] }
    w.notes = []
    Object.defineProperty(window, 'Notification', {
      value: class {
        static permission = 'granted'
        static requestPermission() {
          return Promise.resolve('granted')
        }
        onclick: (() => void) | null = null
        constructor(title: string, opts: { body: string; tag: string }) {
          w.notes.push(`${title} | ${opts.body} | ${opts.tag}`)
        }
        close() {
          return undefined
        }
      },
    })
  })
  writeFileSync(
    statsPath,
    JSON.stringify({
      supported: true,
      at: '2026-11-13T00:00:00Z',
      cpu: { load: [1, 1, 1], cores: 12 },
      memory: { total: 36 * GB, available: 12 * GB, pressure: 'normal' },
      swap: { total: 0, used: 0 },
      disks: [{ mount: '/', total: 460 * GB, free: 2 * GB }],
    }),
  )
  await page.goto(`${base}/#t=${token}`)
  const notes = () => page.evaluate(() => (window as unknown as { notes: string[] }).notes)
  await expect.poll(notes).toEqual(['Disk almost full | 2.0 GB free on /. Builds and agents may fail. | system:disk:/'])
  // Later samples at the same level do not notify again.
  await page.waitForTimeout(5500)
  expect(await notes()).toHaveLength(1)
})
