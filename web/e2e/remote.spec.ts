import { expect, test, type Page } from '@playwright/test'
import { port, token } from '../playwright.config'

const base = `http://127.0.0.1:${String(port)}`
const remotePort = 7421
const remoteBase = `http://127.0.0.1:${String(remotePort)}`

// setRemote changes the settings through the local API, as the desk does.
async function setRemote(page: Page, settings: object) {
  const res = await page.request.post(`${base}/api/remote`, {
    headers: { Authorization: `Bearer ${token}`, Origin: base },
    data: settings,
  })
  expect(res.ok()).toBe(true)
}

const on = (level: string) => ({ enabled: true, level, host: `127.0.0.1:${String(remotePort)}`, port: remotePort, tailscaleLogins: ['tester@example.com'] })

test.afterEach(async ({ page }) => {
  await setRemote(page, { enabled: false, level: 'answer', host: '', port: remotePort, tailscaleLogins: [] })
})

test('remote access is set on the desk and limits a remote device to its level', async ({ page, browser }) => {
  // Turn it on in Settings, on the desk.
  await page.goto(`${base}/#t=${token}`)
  await page.getByRole('navigation', { name: 'Sessions' }).getByRole('button', { name: 'Settings' }).click()
  const settings = page.getByRole('dialog', { name: 'Settings' })
  await settings.getByLabel('Allow remote access').check()
  await settings.getByLabel('Allowed Tailscale logins').fill('tester@example.com')
  await settings.getByLabel('Host name remote devices use').fill(`127.0.0.1:${String(remotePort)}`)
  await settings.getByLabel('Local port').fill(String(remotePort))
  await settings.getByRole('button', { name: 'Save' }).click()
  await expect(settings.getByText('Saved.')).toBeVisible()
  await expect(settings).toContainText(`http://127.0.0.1:${String(remotePort)}`)

  // A device without Tailscale's vouching gets nothing.
  const stranger = await browser.newContext()
  const denied = await stranger.request.get(`${remoteBase}/api/sessions`)
  expect(denied.status()).toBe(401)
  await stranger.close()

  // A vouched-for device reads, sees its level, and cannot reply or manage.
  const phone = await browser.newContext({ extraHTTPHeaders: { 'Tailscale-User-Login': 'tester@example.com' } })
  const remote = await phone.newPage()
  await remote.goto(`${remoteBase}/s/a0000011`)
  const nav = remote.getByRole('navigation', { name: 'Sessions' })
  await expect(nav.getByText('Remote · Read and answer · tester@example.com')).toBeVisible()
  await expect(nav.getByRole('button', { name: 'New session', exact: true })).toHaveCount(0)
  const main = remote.getByRole('main')
  await expect(main.getByRole('heading', { name: 'reviewer' })).toBeVisible()
  await expect(main.getByText('Replies are off for remote devices at this level.')).toBeVisible()
  await expect(main.getByRole('button', { name: 'Stop' })).toHaveCount(0)
  const send = await remote.request.post(`${remoteBase}/api/sessions/a0000011/send`, { headers: { Origin: remoteBase }, data: { text: 'hi' } })
  expect(send.status()).toBe(403)
  // Settings on a remote device show display options only.
  await nav.getByRole('button', { name: 'Settings' }).click()
  await expect(remote.getByRole('dialog', { name: 'Settings' }).getByText('Remote access')).toHaveCount(0)

  // Raised on the desk to Reply, the device can reply but still not manage.
  await setRemote(page, on('reply'))
  await remote.goto(`${remoteBase}/s/a0000011`)
  await expect(nav.getByText('Remote · Reply · tester@example.com')).toBeVisible()
  await expect(remote.getByRole('textbox', { name: 'Reply to reviewer' })).toBeVisible()
  await expect(remote.getByRole('main').getByRole('button', { name: 'Stop' })).toHaveCount(0)
  // And it can never change the settings.
  const change = await remote.request.post(`${remoteBase}/api/remote`, { headers: { Origin: remoteBase }, data: on('manage') })
  expect(change.status()).toBe(403)
  await phone.close()
})
