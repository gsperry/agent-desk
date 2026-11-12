import { memo, type ReactNode } from 'react'
import ReactMarkdown, { type Components } from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { remarkBreaks } from './breaks'
import { parsePathRef } from '../model/paths'
import { useOpenPath } from '../session-context'
import { CodeBlock } from './CodeBlock'

interface HastNode {
  type: string
  value?: string
  tagName?: string
  properties?: Record<string, unknown>
  children?: HastNode[]
}

function textOf(node: HastNode): string {
  if (node.type === 'text') return node.value ?? ''
  return (node.children ?? []).map(textOf).join('')
}

const components: Components = {
  // Fenced code arrives as <pre><code class="language-x">; inline code is
  // a bare <code> and keeps the default rendering.
  pre({ node }) {
    const code = (node as HastNode | undefined)?.children?.find((c) => c.tagName === 'code')
    if (!code) return null
    const cls = code.properties?.className
    const classes = Array.isArray(cls) ? cls.map(String) : []
    const lang = classes.find((c) => c.startsWith('language-'))?.slice('language-'.length)
    return <CodeBlock code={textOf(code).replace(/\n$/, '')} lang={lang} />
  },
  code({ children }) {
    return <InlineCode>{children}</InlineCode>
  },
  a({ href, children }) {
    return (
      <a href={href} target="_blank" rel="noreferrer noopener">
        {children}
      </a>
    )
  },
}

// InlineCode turns code that names a file into a link that opens it.
function InlineCode({ children }: { children?: ReactNode }) {
  const openPath = useOpenPath()
  const text = typeof children === 'string' ? children : ''
  const ref = openPath && text ? parsePathRef(text) : null
  if (!ref || !openPath) return <code>{children}</code>
  return (
    <button
      type="button"
      className="path-link"
      title={`Open in VS Code${ref.line ? ` at line ${String(ref.line)}` : ''}`}
      onClick={() => {
        openPath(ref)
      }}
    >
      <code>{children}</code>
    </button>
  )
}

export const Markdown = memo(function Markdown({ text }: { text: string }) {
  return (
    <div className="markdown">
      <ReactMarkdown remarkPlugins={[remarkGfm, remarkBreaks]} components={components}>
        {text}
      </ReactMarkdown>
    </div>
  )
})
