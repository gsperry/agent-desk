// Notifier wraps the browser Notification API so the store can be tested
// without one. 127.0.0.1 counts as a secure context, so it is available.
// Inside the desktop app the web view has no Notification API; the app
// grants two commands instead (desktop/capabilities/page.json).
export type Permission = NotificationPermission | 'unsupported'

export interface Notifier {
  permission(): Permission
  request(): Promise<Permission>
  show(title: string, body: string, tag: string, onClick: () => void): void
}

export const browserNotifier: Notifier = {
  permission() {
    return typeof Notification === 'undefined' ? 'unsupported' : Notification.permission
  },
  async request() {
    if (typeof Notification === 'undefined') return 'unsupported'
    return Notification.requestPermission()
  },
  show(title, body, tag, onClick) {
    if (typeof Notification === 'undefined' || Notification.permission !== 'granted') return
    const n = new Notification(title, { body, tag })
    n.onclick = () => {
      window.focus()
      onClick()
      n.close()
    }
  },
}

type Invoke = (cmd: string, args: Record<string, unknown>) => Promise<unknown>

interface DesktopWindow {
  __TAURI__?: { core?: { invoke?: Invoke } }
}

/** desktopInvoke returns the desktop app's command bridge, or undefined in a browser. */
export function desktopInvoke(w: unknown = globalThis): Invoke | undefined {
  const invoke = (w as DesktopWindow).__TAURI__?.core?.invoke
  return typeof invoke === 'function' ? invoke : undefined
}

/**
 * desktopNotifier shows native notifications through the app. macOS asks
 * for permission on the first one, so the page treats them as granted; the
 * user still turns them on in the sidebar. Clicking one brings the app
 * forward but does not open the session.
 */
export function desktopNotifier(invoke: Invoke): Notifier {
  return {
    permission: () => 'granted',
    request: () => Promise.resolve('granted'),
    show(title, body) {
      invoke('notify', { title, body }).catch(() => undefined)
    },
  }
}

/** defaultNotifier picks the desktop bridge when present. */
export function defaultNotifier(): Notifier {
  const invoke = desktopInvoke()
  return invoke ? desktopNotifier(invoke) : browserNotifier
}

/** setBadge shows the attention count on the dock icon in the desktop app. */
export function setBadge(count: number, invoke: Invoke | undefined = desktopInvoke()): void {
  if (!invoke) return
  invoke('set_badge', { count: Math.max(0, Math.floor(count)) }).catch(() => undefined)
}
