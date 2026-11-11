import type { ReactNode } from 'react'
import { defaultSnippets, validName, type Snippet } from '../model/snippets'
import { newRule, ruleStatuses, type Rule, type RuleStatus } from '../model/rules'
import type { SummarySettings } from '../model/summaries'
import { useRemote } from '../remote-context'
import { RemoteAccess, type RemoteSettings } from './RemoteAccess'
import type { Template } from '../model/templates'
import { fontSizes, type EditsOpen, type Open, type Theme, type ViewPrefs, type Width } from '../model/viewprefs'

interface Props {
  prefs: ViewPrefs
  onChange: (p: ViewPrefs) => void
  snippets: Snippet[]
  onSnippets: (s: Snippet[]) => void
  templates: Template[]
  onTemplates: (t: Template[]) => void
  rules: Rule[]
  onRules: (r: Rule[]) => void
  summaries: SummarySettings
  onSummaries: (s: SummarySettings) => void
  loadRemote: () => Promise<{ data?: RemoteSettings; available: boolean; error?: string }>
  saveRemote: (s: RemoteSettings) => Promise<{ data?: RemoteSettings; error?: string }>
  onClose: () => void
}

function Segmented<T extends string>({ label, value, options, onChange }: { label: string; value: T; options: [T, string][]; onChange: (v: T) => void }) {
  return (
    <div className="setting">
      <span className="setting-label">{label}</span>
      <div className="segmented" role="radiogroup" aria-label={label}>
        {options.map(([v, text]) => (
          <button
            key={v}
            type="button"
            role="radio"
            aria-checked={v === value}
            className={v === value ? 'active' : undefined}
            onClick={() => {
              onChange(v)
            }}
          >
            {text}
          </button>
        ))}
      </div>
    </div>
  )
}

const openClosed: [Open, string][] = [
  ['closed', 'Collapsed'],
  ['open', 'Expanded'],
]

export function Settings({ prefs, onChange, snippets, onSnippets, templates, onTemplates, rules, onRules, summaries, onSummaries, loadRemote, saveRemote, onClose }: Props) {
  const onDesk = !useRemote().remote
  const setRule = (id: string, p: Partial<Rule>) => {
    onRules(rules.map((r) => (r.id === id ? { ...r, ...p } : r)))
  }
  const set = (p: Partial<ViewPrefs>) => {
    onChange({ ...prefs, ...p })
  }
  const collapse = (p: Partial<ViewPrefs['collapse']>) => {
    onChange({ ...prefs, collapse: { ...prefs.collapse, ...p } })
  }
  return (
    <div className="overlay" onMouseDown={onClose}>
      <div
        className="settings"
        role="dialog"
        aria-label="Settings"
        onMouseDown={(e) => {
          e.stopPropagation()
        }}
        onKeyDown={(e) => {
          if (e.key === 'Escape') onClose()
        }}
      >
        <header className="settings-head">
          <h2>Settings</h2>
          <button type="button" className="link-button" onClick={onClose} autoFocus>
            Done
          </button>
        </header>
        <Section title="Appearance">
          <Segmented<Theme>
            label="Theme"
            value={prefs.theme}
            options={[
              ['system', 'System'],
              ['light', 'Light'],
              ['dark', 'Dark'],
            ]}
            onChange={(theme) => {
              set({ theme })
            }}
          />
          <div className="setting">
            <span className="setting-label">Font size</span>
            <div className="stepper">
              <button
                type="button"
                aria-label="Smaller text"
                disabled={prefs.fontSize <= fontSizes.min}
                onClick={() => {
                  set({ fontSize: prefs.fontSize - 1 })
                }}
              >
                A−
              </button>
              <span aria-live="polite">{prefs.fontSize}px</span>
              <button
                type="button"
                aria-label="Larger text"
                disabled={prefs.fontSize >= fontSizes.max}
                onClick={() => {
                  set({ fontSize: prefs.fontSize + 1 })
                }}
              >
                A+
              </button>
            </div>
          </div>
          <Segmented<Width>
            label="Reading width"
            value={prefs.width}
            options={[
              ['narrow', 'Narrow'],
              ['medium', 'Medium'],
              ['wide', 'Wide'],
              ['full', 'Full'],
            ]}
            onChange={(width) => {
              set({ width })
            }}
          />
        </Section>
        <Section title="Start collapsed or expanded">
          <Segmented<EditsOpen>
            label="Edits and writes"
            value={prefs.collapse.edits}
            options={[
              ['closed', 'Collapsed'],
              ['auto', 'Small diffs'],
              ['open', 'Expanded'],
            ]}
            onChange={(edits) => {
              collapse({ edits })
            }}
          />
          <Segmented<Open>
            label="Shell commands"
            value={prefs.collapse.bash}
            options={openClosed}
            onChange={(bash) => {
              collapse({ bash })
            }}
          />
          <Segmented<Open>
            label="Reads and searches"
            value={prefs.collapse.read}
            options={openClosed}
            onChange={(read) => {
              collapse({ read })
            }}
          />
          <Segmented<Open>
            label="Subagents"
            value={prefs.collapse.agents}
            options={openClosed}
            onChange={(agents) => {
              collapse({ agents })
            }}
          />
          <Segmented<Open>
            label="Other tools"
            value={prefs.collapse.other}
            options={openClosed}
            onChange={(other) => {
              collapse({ other })
            }}
          />
          <Segmented<Open>
            label="Thinking"
            value={prefs.collapse.thinking}
            options={openClosed}
            onChange={(thinking) => {
              collapse({ thinking })
            }}
          />
          <p className="muted setting-note">Failed tool calls always start expanded. Changes apply to blocks as they appear.</p>
        </Section>
        {onDesk && (
          <>
        <Section title="Reply snippets">
          <p className="muted setting-note">
            Insert from the composer's ⋯ menu, or type /name and press Tab. {'{session}'}, {'{branch}'}, {'{project}'} and {'{directory}'} are filled in.
          </p>
          {snippets.map((sn, i) => (
            <div key={i} className="snippet-edit">
              <input
                aria-label="Snippet name"
                value={sn.name}
                className={validName(sn.name) ? undefined : 'invalid'}
                onChange={(e) => {
                  onSnippets(snippets.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)))
                }}
              />
              <textarea
                aria-label="Snippet text"
                rows={2}
                value={sn.text}
                onChange={(e) => {
                  onSnippets(snippets.map((x, j) => (j === i ? { ...x, text: e.target.value } : x)))
                }}
              />
              <button
                type="button"
                className="link-button"
                onClick={() => {
                  onSnippets(snippets.filter((_, j) => j !== i))
                }}
              >
                Remove
              </button>
            </div>
          ))}
          <div className="snippet-actions">
            <button
              type="button"
              className="link-button"
              onClick={() => {
                onSnippets([...snippets, { name: `snippet${String(snippets.length + 1)}`, text: '' }])
              }}
            >
              Add snippet
            </button>
            <button
              type="button"
              className="link-button"
              onClick={() => {
                onSnippets(defaultSnippets)
              }}
            >
              Restore defaults
            </button>
          </div>
        </Section>
        <Section title="Rules">
          <p className="muted setting-note">
            When a session reaches a status, notify you, suggest a reply, or both. A suggested reply shows above the composer and is only sent when you press Send. Rules run while an Agent Desk tab is open.
          </p>
          {rules.map((r) => (
            <fieldset key={r.id} className="rule-edit" aria-label={`Rule ${r.name}`}>
              <div className="rule-line">
                <input
                  type="checkbox"
                  aria-label="Enabled"
                  checked={r.enabled}
                  onChange={(e) => {
                    setRule(r.id, { enabled: e.target.checked })
                  }}
                />
                <input
                  aria-label="Rule name"
                  value={r.name}
                  onChange={(e) => {
                    setRule(r.id, { name: e.target.value })
                  }}
                />
                <button
                  type="button"
                  className="link-button"
                  aria-label={`Remove rule ${r.name}`}
                  onClick={() => {
                    onRules(rules.filter((x) => x.id !== r.id))
                  }}
                >
                  Remove
                </button>
              </div>
              <div className="rule-line">
                <span className="muted">When a session matching</span>
                <input
                  aria-label="Match"
                  placeholder="any session"
                  value={r.match}
                  onChange={(e) => {
                    setRule(r.id, { match: e.target.value })
                  }}
                />
                <select
                  aria-label="Status"
                  value={r.status}
                  onChange={(e) => {
                    setRule(r.id, { status: e.target.value as RuleStatus })
                  }}
                >
                  {ruleStatuses.map(([v, label]) => (
                    <option key={v} value={v}>
                      {label}
                    </option>
                  ))}
                </select>
              </div>
              <label className="check">
                <input
                  type="checkbox"
                  checked={r.notify}
                  onChange={(e) => {
                    setRule(r.id, { notify: e.target.checked })
                  }}
                />{' '}
                Notify me
              </label>
              <textarea
                aria-label="Suggested reply"
                rows={2}
                placeholder="Suggested reply (optional). {session}, {branch}, {project} and {directory} are filled in."
                value={r.prompt}
                onChange={(e) => {
                  setRule(r.id, { prompt: e.target.value })
                }}
              />
            </fieldset>
          ))}
          <button
            type="button"
            className="link-button"
            onClick={() => {
              onRules([...rules, newRule(`r${String(Date.now())}`)])
            }}
          >
            Add rule
          </button>
        </Section>
        <Section title="Catch-up summaries">
          <p className="muted setting-note">
            Adds a “Catch me up” button to sessions. It runs your own Claude Code CLI (<code>claude -p</code>) on what happened since you last looked, with no tools.{' '}
            <strong>Each summary costs tokens on your Claude Code provider.</strong> Nothing runs until you press the button.
          </p>
          <label className="check">
            <input
              type="checkbox"
              checked={summaries.enabled}
              onChange={(e) => {
                onSummaries({ ...summaries, enabled: e.target.checked })
              }}
            />{' '}
            Allow catch-up summaries
          </label>
          <label className="field summary-model">
            <span>Model</span>
            <input
              value={summaries.model}
              placeholder="haiku"
              onChange={(e) => {
                onSummaries({ ...summaries, model: e.target.value })
              }}
            />
          </label>
        </Section>
        <Section title="Session templates">
          <p className="muted setting-note">Save one from the new session form with “Save as template”; pick it at the top of the form next time.</p>
          {templates.length === 0 && <p className="muted setting-note">No templates yet.</p>}
          <ul className="template-list">
            {templates.map((t) => (
              <li key={t.name}>
                <strong>{t.name}</strong>
                <span className="muted">{templateSummary(t)}</span>
                <button
                  type="button"
                  className="link-button"
                  aria-label={`Remove template ${t.name}`}
                  onClick={() => {
                    onTemplates(templates.filter((x) => x.name !== t.name))
                  }}
                >
                  Remove
                </button>
              </li>
            ))}
          </ul>
        </Section>
            <Section title="Remote access">
              <RemoteAccess load={loadRemote} save={saveRemote} />
            </Section>
          </>
        )}
      </div>
    </div>
  )
}

function templateSummary(t: Template): string {
  return [t.tool, t.directory, t.group && `group ${t.group}`, t.worktree && 'worktree', t.model, t.effort, t.prompt && 'with prompt'].filter(Boolean).join(' · ')
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="settings-section">
      <h3>{title}</h3>
      {children}
    </section>
  )
}
