import { createContext, useContext } from 'react'
import { defaultSnippets, type Snippet } from './model/snippets'

export const SnippetsContext = createContext<Snippet[]>(defaultSnippets)

export function useSnippets(): Snippet[] {
  return useContext(SnippetsContext)
}
