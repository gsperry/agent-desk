import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { Markdown } from './Markdown'

describe('Markdown line breaks', () => {
  it('keeps a single line break in prose', () => {
    const html = renderToStaticMarkup(<Markdown text={'Delivery date: [you]\nOwner: [you]'} />)
    expect(html).toMatch(/Delivery date: \[you\]<br\/>\s*Owner: \[you\]/)
  })

  it('leaves code blocks and paragraphs alone', () => {
    const html = renderToStaticMarkup(<Markdown text={'one\n\ntwo\n\n```\na\nb\n```'} />)
    expect(html).toContain('<p>one</p>')
    expect(html).toContain('<p>two</p>')
    expect(html).not.toContain('a<br/>b')
  })
})
