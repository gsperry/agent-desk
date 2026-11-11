import { useCallback, useEffect, useMemo, useState } from 'react'
import { takeToken } from './auth'
import { SessionView } from './components/SessionView'
import { Sidebar, type ViewOptions } from './components/Sidebar'
import { ConfirmDialog } from './components/Dialog'
import { NewGroup } from './components/NewGroup'
import { NewSession } from './components/NewSession'
import { ConversationView, HistoryList } from './components/History'
import { Overview } from './components/Overview'
import { SearchView } from './components/SearchView'
import { Settings } from './components/Settings'
import { UsageView } from './components/UsageView'
import { Switcher } from './components/Switcher'
import { attentionCount, nextAttention, ordered } from './model/attention'
import { groupSessions } from './model/sidebar'
import type { Conflicts } from './model/conflicts'
import { conversationTitle, sessionsByConversation } from './model/history'
import type { PathRef } from './model/paths'
import { loadSnippets, saveSnippets, type Snippet } from './model/snippets'
import { loadTemplates, saveTemplates, upsertTemplate, type Template } from './model/templates'
import { loadSummaries, saveSummaries, type SummarySettings } from './model/summaries'
import { directorySuggestions, resumeDefaults, spawnDefaults } from './model/spawn'
import { loadViewPrefs, saveViewPrefs, widths, type ViewPrefs } from './model/viewprefs'
import { Store, useStore } from './store'
import type { Answer, Conversation, Project, SessionAction, SpawnRequest } from './types'
import { SnippetsContext } from './snippets-context'
import { ViewPrefsContext } from './viewprefs-context'

const token = takeToken()

export default function App() {
  if (!token) return <NoToken />
  return <Desk token={token} />
}

function sessionFromPath(): string | null {
  const m = /^\/s\/([^/]+)/.exec(window.location.pathname)
  return m?.[1] ? decodeURIComponent(m[1]) : null
}

// historyFromPath reads /history (the list) and /history/<id> (one
// conversation); null means the page is not in history.
function historyFromPath(): { id: string | null } | null {
  const p = window.location.pathname
  if (p === '/history' || p === '/history/') return { id: null }
  const m = /^\/history\/([^/]+)/.exec(p)
  return m?.[1] ? { id: decodeURIComponent(m[1]) } : null
}

function usageFromPath(): boolean {
  return window.location.pathname === '/usage'
}

const noPending: never[] = []

function isTyping(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)
}

function Desk({ token }: { token: string }) {
  const store = useMemo(() => new Store(token), [token])
  useEffect(() => store.start(), [store])
  const { conn, data, pending, acks, prefs, permission, toast, seen, rules, suggestions } = useStore(store)
  const [selected, setSelected] = useState<string | null>(sessionFromPath)
  const [historyRoute, setHistoryRoute] = useState<{ id: string | null } | null>(historyFromPath)
  const [conversations, setConversations] = useState<Conversation[] | null>(null)
  const [usageOpen, setUsageOpen] = useState(usageFromPath)
  const [view, setView] = useState<ViewOptions>({ showArchived: false, onlyAttention: false })
  const [switcher, setSwitcher] = useState(false)
  const [visible, setVisible] = useState(document.visibilityState === 'visible')
  const [viewPrefs, setViewPrefs] = useState<ViewPrefs>(() => loadViewPrefs(localStorage))
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [snippets, setSnippets] = useState<Snippet[]>(() => loadSnippets(localStorage))
  useEffect(() => {
    saveSnippets(localStorage, snippets)
  }, [snippets])
  const [summaries, setSummaries] = useState<SummarySettings>(() => loadSummaries(localStorage))
  useEffect(() => {
    saveSummaries(localStorage, summaries)
  }, [summaries])
  const [templates, setTemplates] = useState<Template[]>(() => loadTemplates(localStorage))
  useEffect(() => {
    saveTemplates(localStorage, templates)
  }, [templates])
  // searchText is the sidebar box; searchQuery is the search being shown,
  // set once the text settles.
  const [searchText, setSearchText] = useState('')
  const [searchQuery, setSearchQuery] = useState<string | null>(null)
  const [searchArchived, setSearchArchived] = useState(false)
  type Modal = { kind: 'spawn'; initial: SpawnRequest; resumeTitle?: string } | { kind: 'group' } | { kind: 'deleteGroup'; path: string; busy: boolean; error: string | null }
  const [modal, setModal] = useState<Modal | null>(null)
  const [conflicts, setConflicts] = useState<Conflicts>({})

  // Shared-file warnings refresh on their own cadence; they depend on
  // transcripts, not session status.
  useEffect(() => {
    let live = true
    const load = () => {
      void store.conflicts().then((c) => {
        if (live && c) setConflicts(c)
      })
    }
    load()
    const t = setInterval(load, 10_000)
    return () => {
      live = false
      clearInterval(t)
    }
  }, [store])

  useEffect(() => {
    const q = searchText.trim()
    const t = setTimeout(() => {
      setSearchQuery(q.length >= 2 ? q : null)
    }, 300)
    return () => {
      clearTimeout(t)
    }
  }, [searchText])

  useEffect(() => {
    saveViewPrefs(localStorage, viewPrefs)
    const root = document.documentElement
    if (viewPrefs.theme === 'system') delete root.dataset.theme
    else root.dataset.theme = viewPrefs.theme
    root.style.setProperty('--font-size', `${String(viewPrefs.fontSize)}px`)
    root.style.setProperty('--reading-width', widths[viewPrefs.width])
  }, [viewPrefs])

  const sessions = useMemo(() => data.sessions?.sessions ?? [], [data.sessions])
  const amGroups = useMemo(() => data.sessions?.groups ?? [], [data.sessions])
  const groups = useMemo(() => groupSessions(sessions, { ...view, acks, groups: amGroups }), [sessions, view, acks, amGroups])
  const directories = useMemo(() => directorySuggestions(sessions, amGroups), [sessions, amGroups])
  // j/k walk every session that needs the human, even when filters hide some.
  const walkOrder = useMemo(() => ordered(groupSessions(sessions, { showArchived: false, acks })), [sessions, acks])
  const attention = attentionCount(sessions, acks)

  const select = useCallback((id: string) => {
    history.pushState(null, '', `/s/${encodeURIComponent(id)}`)
    setSelected(id)
    setHistoryRoute(null)
    setUsageOpen(false)
    setSearchText('')
    setSearchQuery(null)
  }, [])

  useEffect(() => {
    const onPop = () => {
      setSelected(sessionFromPath())
      setHistoryRoute(historyFromPath())
      setUsageOpen(usageFromPath())
    }
    const onVisibility = () => {
      setVisible(document.visibilityState === 'visible')
    }
    window.addEventListener('popstate', onPop)
    document.addEventListener('visibilitychange', onVisibility)
    return () => {
      window.removeEventListener('popstate', onPop)
      document.removeEventListener('visibilitychange', onVisibility)
    }
  }, [])

  useEffect(() => {
    store.setOnOpen(select)
    store.setViewing(selected, visible)
  }, [store, select, selected, visible])

  // The open session counts as read while the page is visible.
  const selectedSession = sessions.find((s) => s.id === selected)
  const lastSeq = selected ? data.docs[selected]?.items.at(-1)?.seq : undefined
  useEffect(() => {
    const conv = selectedSession?.agentSessionId
    const at = selectedSession?.lastActivity
    if (!visible || !conv || !at || lastSeq === undefined) return
    store.markViewed(conv, at, lastSeq)
  }, [store, visible, selectedSession?.agentSessionId, selectedSession?.lastActivity, lastSeq])

  useEffect(() => {
    document.title = attention > 0 ? `(${String(attention)}) Agent Desk` : 'Agent Desk'
  }, [attention])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        setSwitcher((open) => !open)
        return
      }
      if (switcher || isTyping(e.target) || e.metaKey || e.ctrlKey || e.altKey) return
      if (e.key === '/') {
        e.preventDefault()
        document.getElementById('search-box')?.focus()
        return
      }
      if (e.key === 'j' || e.key === 'k') {
        const id = nextAttention(walkOrder, acks, selected, e.key === 'j' ? 1 : -1)
        if (id) {
          e.preventDefault()
          select(id)
        }
      }
    }
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('keydown', onKey)
    }
  }, [switcher, walkOrder, acks, selected, select])

  const follow = useCallback((id: string) => {
    store.follow(id)
  }, [store])
  const unfollow = useCallback((id: string) => {
    store.unfollow(id)
  }, [store])
  const loadOlder = useCallback((id: string) => {
    void store.loadOlder(id)
  }, [store])
  const send = useCallback((id: string, text: string) => {
    void store.send(id, text)
  }, [store])
  const dismiss = useCallback((id: string, clientId: string) => {
    store.dismiss(id, clientId)
  }, [store])
  const readScreen = useCallback((id: string) => store.screen(id), [store])
  const readStyled = useCallback((id: string) => store.styledScreen(id), [store])
  const answer = useCallback((id: string, a: Answer) => store.answer(id, a), [store])
  const loadOptions = useCallback(() => store.spawnOptions(), [store])
  const readAgents = useCallback((id: string) => store.agents(id), [store])
  const loadOverview = useCallback(() => store.overview(), [store])
  const pasteImage = useCallback((image: Blob) => store.paste(image), [store])
  const readChanges = useCallback((id: string, mode: 'uncommitted' | 'branch') => store.changes(id, mode), [store])
  const goHome = useCallback(() => {
    history.pushState(null, '', '/')
    setSelected(null)
    setHistoryRoute(null)
    setUsageOpen(false)
    setSearchText('')
    setSearchQuery(null)
  }, [])
  const openHistory = useCallback((id: string | null) => {
    history.pushState(null, '', id ? `/history/${encodeURIComponent(id)}` : '/history')
    setSelected(null)
    setHistoryRoute({ id })
    setUsageOpen(false)
    setSearchText('')
    setSearchQuery(null)
  }, [])
  const loadConversations = useCallback(async () => {
    const r = await store.conversations()
    if (r.data) setConversations(r.data)
    return r
  }, [store])
  const catchUp = useCallback((id: string, since: number) => store.catchUp(id, since, summaries.model), [store, summaries.model])
  const dismissSuggestion = useCallback((id: string) => {
    store.dismissSuggestion(id)
  }, [store])
  const exportConversation = useCallback((id: string) => {
    void store.exportConversation(id)
  }, [store])
  const openUsage = useCallback(() => {
    history.pushState(null, '', '/usage')
    setSelected(null)
    setHistoryRoute(null)
    setUsageOpen(true)
    setSearchText('')
    setSearchQuery(null)
  }, [])
  const readUsage = useCallback((days: number) => store.usage(days), [store])
  const readConversation = useCallback((id: string, before?: number) => store.conversationPage(id, before), [store])
  const owners = useMemo(() => sessionsByConversation(sessions), [sessions])
  // A conversation opened by URL still needs its listing for the header.
  const historyId = historyRoute?.id
  const needListing = Boolean(historyId) && conversations === null
  useEffect(() => {
    if (!needListing) return
    let live = true
    void store.conversations().then((r) => {
      if (live && r.data) setConversations(r.data)
    })
    return () => {
      live = false
    }
  }, [needListing, store])
  const readAgentTranscript = useCallback((id: string, agentId: string) => store.agentTranscript(id, agentId), [store])
  const spawn = useCallback((r: SpawnRequest) => store.spawn(r), [store])
  const act = useCallback((id: string, a: SessionAction) => store.act(id, a), [store])
  const openSpawn = useCallback((project: Project | null) => {
    setModal({ kind: 'spawn', initial: spawnDefaults(project, '', false) })
  }, [])
  const runSearch = useCallback((q: string, archived: boolean) => store.search(q, archived), [store])
  const around = useCallback((id: string, uuid: string) => store.around(id, uuid), [store])
  const openPath = useCallback(
    (id: string, ref: PathRef) => {
      void store.openPath(id, ref)
    },
    [store],
  )
  useEffect(() => {
    if (!toast) return
    const t = setTimeout(() => {
      store.clearToast(toast.id)
    }, 5000)
    return () => {
      clearTimeout(t)
    }
  }, [store, toast])

  if (conn.kind === 'unauthorized') return <NoToken />

  const session = sessions.find((s) => s.id === selected)
  return (
    <ViewPrefsContext.Provider value={viewPrefs}>
    <SnippetsContext.Provider value={snippets}>
        <div className="desk">
          <Sidebar
            state={data.sessions}
            conn={conn}
            groups={groups}
            acks={acks}
            seen={seen}
            conflicts={conflicts}
            attention={attention}
            view={view}
            onView={setView}
            selected={selected}
            onSelect={select}
            onStartManager={() => store.startManager()}
            prefs={prefs}
            permission={permission}
            onPrefs={(p) => {
              store.setPrefs(p)
            }}
            onRequestPermission={() => {
              void store.requestPermission()
            }}
            onOpenSettings={() => {
              setSettingsOpen(true)
            }}
            search={searchText}
            onSearch={setSearchText}
            onNewSession={openSpawn}
            onNewGroup={() => {
              setModal({ kind: 'group' })
            }}
            onDeleteGroup={(path) => {
              setModal({ kind: 'deleteGroup', path, busy: false, error: null })
            }}
            onHome={goHome}
            onHistory={() => {
              openHistory(null)
            }}
            historyOpen={historyRoute !== null}
            onUsage={openUsage}
            usageOpen={usageOpen}
          />
          {searchQuery ? (
            <SearchView
              query={searchQuery}
              archived={searchArchived}
              onArchived={setSearchArchived}
              run={runSearch}
              around={around}
              openPath={openPath}
              onOpenSession={select}
              onClose={() => {
                setSearchText('')
                setSearchQuery(null)
              }}
            />
          ) : usageOpen ? (
            <UsageView load={readUsage} conversations={conversations} loadConversations={loadConversations} onOpenConversation={openHistory} />
          ) : historyRoute ? (
            historyRoute.id ? (
              <ConversationView
                key={historyRoute.id}
                id={historyRoute.id}
                conversation={conversations?.find((c) => c.id === historyRoute.id)}
                owner={owners.get(historyRoute.id)}
                load={readConversation}
                onBack={() => {
                  openHistory(null)
                }}
                onOpenSession={select}
                onExport={exportConversation}
                onResume={(c) => {
                  setModal({ kind: 'spawn', initial: resumeDefaults(c.id, c.directory ?? ''), resumeTitle: conversationTitle(c) })
                }}
              />
            ) : (
              <HistoryList load={loadConversations} owners={owners} onOpen={openHistory} />
            )
          ) : selected ? (
            data.sessions ? (
              <SessionView
                key={selected}
                session={session}
                sessionId={selected}
                doc={data.docs[selected]}
                pending={pending[selected] ?? noPending}
                connected={conn.kind === 'open'}
                insideSession={data.sessions.insideSession}
                follow={follow}
                unfollow={unfollow}
                loadOlder={loadOlder}
                send={send}
                dismiss={dismiss}
                readScreen={readScreen}
                readStyled={readStyled}
                answer={answer}
                openPath={openPath}
                groups={amGroups}
                act={act}
                seenMark={session?.agentSessionId ? seen[session.agentSessionId] : undefined}
                readAgents={readAgents}
                readAgentTranscript={readAgentTranscript}
                readChanges={readChanges}
                conflicts={selected ? (conflicts[selected] ?? []) : []}
                sessions={sessions}
                pasteImage={pasteImage}
                exportConversation={exportConversation}
                suggestion={suggestions[selected]}
                catchUp={summaries.enabled ? catchUp : undefined}
                dismissSuggestion={dismissSuggestion}
              />
            ) : (
              <main className="main">
                <div className="empty">Loading sessions…</div>
              </main>
            )
          ) : (
            <Overview sessions={sessions} acks={acks} seen={seen} load={loadOverview} onOpen={select} />
          )}
          {modal?.kind === 'spawn' && (
            <NewSession
              initial={modal.initial}
              resumeTitle={modal.resumeTitle}
              templates={templates}
              onSaveTemplate={(t) => {
                setTemplates((list) => upsertTemplate(list, t))
              }}
              directories={directories}
              loadOptions={loadOptions}
              spawn={spawn}
              onStarted={(id) => {
                setModal(null)
                select(id)
              }}
              onClose={() => {
                setModal(null)
              }}
            />
          )}
          {modal?.kind === 'group' && (
            <NewGroup
              directories={directories}
              create={(path, directory) => store.createGroup(path, directory)}
              onClose={() => {
                setModal(null)
              }}
            />
          )}
          {modal?.kind === 'deleteGroup' && (
            <ConfirmDialog
              title={`Delete group ${modal.path}?`}
              message="The group is removed. Sessions in it move to the root and keep running."
              confirm="Delete group"
              danger
              busy={modal.busy}
              error={modal.error}
              onConfirm={() => {
                setModal({ ...modal, busy: true })
                void store.deleteGroup(modal.path).then((error) => {
                  setModal(error ? { ...modal, busy: false, error } : null)
                })
              }}
              onClose={() => {
                setModal(null)
              }}
            />
          )}
          {toast && (
            <div className="toast" role="status">
              {toast.message}
            </div>
          )}
          {settingsOpen && (
            <Settings
              prefs={viewPrefs}
              onChange={setViewPrefs}
              snippets={snippets}
              onSnippets={setSnippets}
              templates={templates}
              onTemplates={setTemplates}
              summaries={summaries}
              onSummaries={setSummaries}
              rules={rules}
              onRules={(r) => {
                store.setRules(r)
              }}
              onClose={() => {
                setSettingsOpen(false)
              }}
            />
          )}
          {switcher && (
            <Switcher
              sessions={sessions}
              acks={acks}
              onPick={select}
              onClose={() => {
                setSwitcher(false)
              }}
            />
          )}
        </div>
    </SnippetsContext.Provider>
    </ViewPrefsContext.Provider>
  )
}

function NoToken() {
  return (
    <main className="shell">
      <h1>Agent Desk</h1>
      <p className="status warn">
        This page needs the link agent-desk printed when it started. Agent Desk makes a new token on every start.
      </p>
    </main>
  )
}
