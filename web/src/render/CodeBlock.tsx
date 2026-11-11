import { useEffect, useState } from 'react'
import { highlight, resolveLang } from './highlight'

interface Props {
  code: string
  lang?: string | null
  // label overrides the language shown in the header, such as a file path.
  label?: string
}

export function CodeBlock({ code, lang, label }: Props) {
  const resolved = resolveLang(lang ?? undefined)
  const [html, setHtml] = useState<{ code: string; html: string } | null>(null)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (!resolved) return
    let live = true
    highlight(code, resolved)
      .then((h) => {
        if (live) setHtml({ code, html: h })
      })
      .catch(() => undefined)
    return () => {
      live = false
    }
  }, [code, resolved])

  const copy = () => {
    void navigator.clipboard.writeText(code).then(() => {
      setCopied(true)
      setTimeout(() => {
        setCopied(false)
      }, 1200)
    })
  }

  return (
    <div className="code-block">
      <div className="code-head">
        <span className="code-lang">{label ?? lang ?? ''}</span>
        <button type="button" className="code-copy" onClick={copy}>
          {copied ? 'Copied' : 'Copy'}
        </button>
      </div>
      {html && html.code === code ? (
        // Shiki escapes the code it highlights.
        <div className="code-body" dangerouslySetInnerHTML={{ __html: html.html }} />
      ) : (
        <pre className="code-body">
          <code>{code}</code>
        </pre>
      )}
    </div>
  )
}
