import type { Nodes, PhrasingContent, Root } from 'mdast'

// remarkBreaks keeps a single line break in prose as a break, as Claude
// Code shows it, instead of joining the lines with a space. Code is left
// alone.
export function remarkBreaks() {
  return (tree: Root) => {
    walk(tree)
  }
}

function walk(node: Nodes): void {
  if (!('children' in node)) return
  const children = node.children as Nodes[]
  for (let i = 0; i < children.length; i++) {
    const child = children[i]
    if (!child) continue
    if (child.type === 'text' && child.value.includes('\n')) {
      const parts: PhrasingContent[] = []
      child.value.split('\n').forEach((line, j) => {
        if (j > 0) parts.push({ type: 'break' })
        if (line) parts.push({ type: 'text', value: line })
      })
      children.splice(i, 1, ...parts)
      i += parts.length - 1
    } else {
      walk(child)
    }
  }
}
