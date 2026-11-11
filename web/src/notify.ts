// Notifier wraps the browser Notification API so the store can be tested
// without one. 127.0.0.1 counts as a secure context, so it is available.
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
