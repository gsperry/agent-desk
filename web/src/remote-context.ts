import { createContext, useContext } from 'react'

export type RemoteLevel = 'answer' | 'reply' | 'manage'

// RemoteInfo says whether this page runs on the desk or on a remote device,
// and what a remote device may do. The server enforces the same rules.
export interface RemoteInfo {
  remote: boolean
  level: RemoteLevel
  user?: string
}

export type Need = 'answer' | 'reply' | 'manage' | 'local'

const rank: Record<RemoteLevel, number> = { answer: 1, reply: 2, manage: 3 }

export const localInfo: RemoteInfo = { remote: false, level: 'manage' }

// can reports whether the page may offer an action that needs n.
export function can(info: RemoteInfo, n: Need): boolean {
  if (!info.remote) return true
  if (n === 'local') return false
  return rank[info.level] >= rank[n]
}

export const levelLabels: Record<RemoteLevel, string> = {
  answer: 'Read and answer',
  reply: 'Reply',
  manage: 'Start and stop',
}

export const RemoteContext = createContext<RemoteInfo>(localInfo)

export function useRemote(): RemoteInfo {
  return useContext(RemoteContext)
}
