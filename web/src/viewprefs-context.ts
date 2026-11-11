import { createContext, useContext } from 'react'
import { defaultViewPrefs, type ViewPrefs } from './model/viewprefs'

export const ViewPrefsContext = createContext<ViewPrefs>(defaultViewPrefs)

export function useViewPrefs(): ViewPrefs {
  return useContext(ViewPrefsContext)
}
