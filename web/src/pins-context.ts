import { createContext, useContext, useState } from 'react'
import { loadNotebook, pinKey, saveNotebook, type Notebook } from './model/pins'
import type { Item } from './types'

// PinsContext lets prompts and replies be pinned in the open conversation.
export interface Pins {
  isPinned: (item: Item) => boolean
  toggle: (item: Item) => void
}

export const PinsContext = createContext<Pins | null>(null)

export function usePins(): Pins | null {
  return useContext(PinsContext)
}

// useNotebook keeps a conversation's note and pins in localStorage.
export function useNotebook(conversation: string | undefined): [Notebook, (nb: Notebook) => void] {
  const [state, setState] = useState(() => ({ conversation, nb: conversation ? loadNotebook(localStorage, conversation) : { note: '', pins: [] } }))
  if (state.conversation !== conversation) {
    setState({ conversation, nb: conversation ? loadNotebook(localStorage, conversation) : { note: '', pins: [] } })
  }
  const set = (nb: Notebook) => {
    if (conversation) saveNotebook(localStorage, conversation, nb)
    setState({ conversation, nb })
  }
  return [state.nb, set]
}

export function pinsFor(nb: Notebook, set: (nb: Notebook) => void, toggle: (nb: Notebook, item: Item) => Notebook): Pins {
  const keys = new Set(nb.pins.map((p) => p.key))
  return {
    isPinned: (item) => {
      const k = pinKey(item)
      return k !== null && keys.has(k)
    },
    toggle: (item) => {
      set(toggle(nb, item))
    },
  }
}
