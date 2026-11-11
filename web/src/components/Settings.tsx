import type { ReactNode } from 'react'
import { fontSizes, type EditsOpen, type Open, type Theme, type ViewPrefs, type Width } from '../model/viewprefs'

interface Props {
  prefs: ViewPrefs
  onChange: (p: ViewPrefs) => void
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

export function Settings({ prefs, onChange, onClose }: Props) {
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
      </div>
    </div>
  )
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="settings-section">
      <h3>{title}</h3>
      {children}
    </section>
  )
}
