import { memo } from 'react'
import ReactMarkdown, { type Components } from 'react-markdown'
import remarkGfm from 'remark-gfm'
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
  a({ href, children }) {
    return (
      <a href={href} target="_blank" rel="noreferrer noopener">
        {children}
      </a>
    )
  },
}

export const Markdown = memo(function Markdown({ text }: { text: string }) {
  return (
    <div className="markdown">
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>
        {text}
      </ReactMarkdown>
    </div>
  )
})
