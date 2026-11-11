import { createContext, useContext } from 'react'

// AgentLinksContext lets an Agent tool call open its subagent's
// transcript: agentFor maps a tool call id to a subagent id.
export interface AgentLinks {
  agentFor: (toolUseId: string) => string | undefined
  open: (agentId: string) => void
}

export const AgentLinksContext = createContext<AgentLinks | null>(null)

export function useAgentLinks(): AgentLinks | null {
  return useContext(AgentLinksContext)
}
