import { createContext, useContext } from 'react'
import type { PathRef } from './model/paths'

// OpenPathContext opens a file from the transcript in the editor. It is
// null outside a session view, where links render as plain code.
export const OpenPathContext = createContext<((ref: PathRef) => void) | null>(null)

export function useOpenPath(): ((ref: PathRef) => void) | null {
  return useContext(OpenPathContext)
}

// SessionDirContext is the session's directory, so paths inside it can be
// shown relative to it.
export const SessionDirContext = createContext<string | undefined>(undefined)

export function useSessionDir(): string | undefined {
  return useContext(SessionDirContext)
}
