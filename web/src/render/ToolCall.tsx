import { useState, type ReactNode } from 'react'
import { diffStats, patchRows, replaceRows, type DiffRow } from '../model/diff'
import { riskyCommand } from '../model/filter'
import { toolPathRef } from '../model/paths'
import { toolLabel } from '../model/transcript'
import { initiallyOpen } from '../model/viewprefs'
import { useAgentLinks } from '../agents-context'
import { useOpenPath } from '../session-context'
import { useViewPrefs } from '../viewprefs-context'
import type { Item } from '../types'
import { CodeBlock } from './CodeBlock'
import { Diff } from './Diff'
import { langForPath } from './highlight'
import { Markdown } from './Markdown'

type Obj = Record<string, unknown>

function obj(v: unknown): Obj {
  return typeof v === 'object' && v !== null && !Array.isArray(v) ? (v as Obj) : {}
}

function str(v: unknown): string {
  return typeof v === 'string' ? v : ''
}

const editTools = new Set(['Edit', 'MultiEdit', 'Write', 'NotebookEdit'])

function editRows(use: Item, result?: Item): DiffRow[] {
  const detail = obj(result?.result?.detail)
  const fromPatch = patchRows(detail.structuredPatch)
  if (fromPatch.length) return fromPatch
  const input = obj(use.tool?.input)
  if (use.tool?.name === 'MultiEdit' && Array.isArray(input.edits)) {
    return input.edits.flatMap((e) => replaceRows(str(obj(e).old_string), str(obj(e).new_string)))
  }
  if (use.tool?.name === 'Write') return []
  return replaceRows(str(input.old_string), str(input.new_string))
}

// newFileStats counts a Write that created a file, which has no patch.
function newFileStats(use: Item): { added: number; removed: number } | null {
  const content = str(obj(use.tool?.input).content)
  return content ? { added: content.replace(/\n$/, '').split('\n').length, removed: 0 } : null
}

interface Props {
  use: Item
  result?: Item
  // live is true while the session runs, so a call without a result is
  // still in progress rather than abandoned.
  live: boolean
}

export function ToolCall({ use, result, live }: Props) {
  const name = use.tool?.name ?? ''
  const label = toolLabel(use)
  const isEdit = editTools.has(name)
  const rows = isEdit ? editRows(use, result) : []
  const stats = isEdit && rows.length ? diffStats(rows) : name === 'Write' ? newFileStats(use) : null
  const failed = result?.result?.isError === true
  const prefs = useViewPrefs()
  const [open, setOpen] = useState(() => initiallyOpen(name, prefs, { failed, diffRows: rows.length }))
  const state = result ? (failed ? 'error' : 'done') : live ? 'running' : 'none'
  const openPath = useOpenPath()
  const risk = name === 'Bash' ? riskyCommand(str(obj(use.tool?.input).command)) : null
  const links = useAgentLinks()
  const subagent = links && use.tool?.id ? links.agentFor(use.tool.id) : undefined
  const fileRef = toolPathRef(obj(use.tool?.input), rows.find((r) => r.newNo !== undefined)?.newNo)

  return (
    <div className={`tool tool-${state}${risk ? ' tool-risky' : ''}`}>
      <button
        type="button"
        className="tool-row"
        aria-expanded={open}
        onClick={() => {
          setOpen(!open)
        }}
      >
        <span className="chevron">{open ? '▾' : '▸'}</span>
        <span className="tool-name">{label.name}</span>
        <span className="tool-summary">{label.summary}</span>
        {risk && (
          <span className="risk-badge" title={`Risky: ${risk}`}>
            ⚠ {risk}
          </span>
        )}
        {stats && (
          <span className="tool-stats">
            <span className="add">+{stats.added}</span> <span className="del">−{stats.removed}</span>
          </span>
        )}
        <span className="tool-state">{state === 'running' ? '…' : state === 'error' ? 'failed' : ''}</span>
      </button>
      {subagent && links && (
        <button
          type="button"
          className="tool-open tool-open-agent"
          title="Read this subagent's transcript"
          onClick={() => {
            links.open(subagent)
          }}
        >
          View transcript
        </button>
      )}
      {fileRef && openPath && (
        <button
          type="button"
          className="tool-open"
          title={`Open ${fileRef.path} in VS Code`}
          onClick={() => {
            openPath(fileRef)
          }}
        >
          Open
        </button>
      )}
      {open && <div className="tool-body">{body(use, result, rows)}</div>}
    </div>
  )
}

function body(use: Item, result: Item | undefined, rows: DiffRow[]): ReactNode {
  const name = use.tool?.name ?? ''
  const input = obj(use.tool?.input)
  const res = result?.result
  const detail = obj(res?.detail)
  switch (name) {
    case 'Bash': {
      const out = str(detail.stdout) || (res?.content ?? '')
      const err = str(detail.stderr)
      return (
        <>
          <CodeBlock code={str(input.command)} lang="bash" />
          {out && <Output text={out} truncated={res?.truncated} />}
          {err && <Output text={err} error />}
          {!res && <p className="muted">No output yet.</p>}
        </>
      )
    }
    case 'Edit':
    case 'MultiEdit':
    case 'NotebookEdit':
      return (
        <>
          <Diff rows={rows} />
          {res?.isError && <Output text={res.content} error />}
        </>
      )
    case 'Write': {
      const path = str(input.file_path)
      return (
        <>
          {rows.length > 0 ? <Diff rows={rows} /> : <CodeBlock code={str(input.content)} lang={langForPath(path)} label={path} />}
          {res?.isError && <Output text={res.content} error />}
        </>
      )
    }
    case 'TodoWrite':
      return <Todos todos={input.todos} />
    case 'AskUserQuestion':
      return <Questions input={input} answers={obj(detail.answers)} answered={res !== undefined} />
    case 'Task':
    case 'Agent':
      return (
        <>
          <Collapsible label="Prompt">
            <Markdown text={str(input.prompt)} />
          </Collapsible>
          {res && (res.isError ? <Output text={res.content} error /> : <Markdown text={res.content} />)}
        </>
      )
    default:
      return (
        <>
          {Object.keys(input).length > 0 && <CodeBlock code={JSON.stringify(input, null, 2)} lang="json" label="input" />}
          {res && <Output text={res.content} error={res.isError} truncated={res.truncated} />}
        </>
      )
  }
}

function Output({ text, error, truncated }: { text: string; error?: boolean; truncated?: boolean }) {
  return (
    <div className={error ? 'output output-error' : 'output'}>
      <pre>{text}</pre>
      {truncated && <p className="muted">Output truncated.</p>}
    </div>
  )
}

function Collapsible({ label, children }: { label: string; children: ReactNode }) {
  const [open, setOpen] = useState(false)
  return (
    <div className="collapsible">
      <button
        type="button"
        className="link-button"
        onClick={() => {
          setOpen(!open)
        }}
      >
        {open ? '▾' : '▸'} {label}
      </button>
      {open && children}
    </div>
  )
}

function Todos({ todos }: { todos: unknown }) {
  if (!Array.isArray(todos)) return null
  return (
    <ul className="todos">
      {todos.map((t, i) => {
        const todo = obj(t)
        const status = str(todo.status)
        return (
          <li key={i} className={`todo todo-${status}`}>
            <span className="todo-mark">{status === 'completed' ? '☑' : status === 'in_progress' ? '◐' : '☐'}</span>
            {str(todo.content) || str(todo.activeForm)}
          </li>
        )
      })}
    </ul>
  )
}

function Questions({ input, answers, answered }: { input: Obj; answers: Obj; answered: boolean }) {
  const questions = Array.isArray(input.questions) ? input.questions.map(obj) : []
  return (
    <div className="questions">
      {!answered && <p className="questions-hint">Not answered yet. Answer with the buttons in the Terminal panel below.</p>}
      {questions.map((q, i) => {
        const question = str(q.question)
        const answer = str(answers[question])
        const options = Array.isArray(q.options) ? q.options.map(obj) : []
        return (
          <div key={i} className="question">
            <p className="question-text">{question}</p>
            <ul>
              {options.map((o, j) => (
                <li key={j} className={str(o.label) === answer ? 'chosen' : undefined}>
                  {str(o.label)}
                  {str(o.description) && <span className="muted"> · {str(o.description)}</span>}
                </li>
              ))}
            </ul>
            {answer && <p className="answer">Answer: {answer}</p>}
          </div>
        )
      })}
    </div>
  )
}
