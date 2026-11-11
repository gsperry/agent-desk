import { createHighlighterCore, type HighlighterCore, type LanguageInput } from 'shiki/core'
import { createJavaScriptRegexEngine } from 'shiki/engine/javascript'

// A curated set keeps the binary small; each grammar loads on first use.
const loaders: Record<string, () => Promise<{ default: LanguageInput }>> = {
  bash: () => import('@shikijs/langs/bash'),
  c: () => import('@shikijs/langs/c'),
  cpp: () => import('@shikijs/langs/cpp'),
  csharp: () => import('@shikijs/langs/csharp'),
  css: () => import('@shikijs/langs/css'),
  diff: () => import('@shikijs/langs/diff'),
  dockerfile: () => import('@shikijs/langs/dockerfile'),
  go: () => import('@shikijs/langs/go'),
  graphql: () => import('@shikijs/langs/graphql'),
  groovy: () => import('@shikijs/langs/groovy'),
  hcl: () => import('@shikijs/langs/hcl'),
  html: () => import('@shikijs/langs/html'),
  ini: () => import('@shikijs/langs/ini'),
  java: () => import('@shikijs/langs/java'),
  javascript: () => import('@shikijs/langs/javascript'),
  json: () => import('@shikijs/langs/json'),
  jsonc: () => import('@shikijs/langs/jsonc'),
  jsx: () => import('@shikijs/langs/jsx'),
  kotlin: () => import('@shikijs/langs/kotlin'),
  lua: () => import('@shikijs/langs/lua'),
  makefile: () => import('@shikijs/langs/makefile'),
  markdown: () => import('@shikijs/langs/markdown'),
  php: () => import('@shikijs/langs/php'),
  powershell: () => import('@shikijs/langs/powershell'),
  properties: () => import('@shikijs/langs/properties'),
  proto: () => import('@shikijs/langs/proto'),
  python: () => import('@shikijs/langs/python'),
  ruby: () => import('@shikijs/langs/ruby'),
  rust: () => import('@shikijs/langs/rust'),
  scala: () => import('@shikijs/langs/scala'),
  scss: () => import('@shikijs/langs/scss'),
  sql: () => import('@shikijs/langs/sql'),
  swift: () => import('@shikijs/langs/swift'),
  toml: () => import('@shikijs/langs/toml'),
  tsx: () => import('@shikijs/langs/tsx'),
  typescript: () => import('@shikijs/langs/typescript'),
  xml: () => import('@shikijs/langs/xml'),
  yaml: () => import('@shikijs/langs/yaml'),
}

const aliases: Record<string, string> = {
  sh: 'bash',
  shell: 'bash',
  shellscript: 'bash',
  zsh: 'bash',
  console: 'bash',
  'c++': 'cpp',
  cs: 'csharp',
  golang: 'go',
  gql: 'graphql',
  js: 'javascript',
  mjs: 'javascript',
  cjs: 'javascript',
  kt: 'kotlin',
  make: 'makefile',
  md: 'markdown',
  ps1: 'powershell',
  py: 'python',
  rb: 'ruby',
  rs: 'rust',
  ts: 'typescript',
  mts: 'typescript',
  yml: 'yaml',
  tf: 'hcl',
  terraform: 'hcl',
  patch: 'diff',
  protobuf: 'proto',
  svg: 'xml',
  htm: 'html',
}

// resolveLang maps a fence info string or file extension to a supported
// grammar, or null to render plain text.
export function resolveLang(lang: string | undefined): string | null {
  if (!lang) return null
  const l = lang.trim().toLowerCase().split(/\s/, 1)[0] ?? ''
  const name = aliases[l] ?? l
  return name in loaders ? name : null
}

// langForPath picks a grammar from a file name.
export function langForPath(path: string): string | null {
  const base = path.split('/').pop() ?? ''
  if (/^dockerfile$/i.test(base)) return 'dockerfile'
  if (/^makefile$/i.test(base)) return 'makefile'
  const ext = base.includes('.') ? base.split('.').pop() : undefined
  return resolveLang(ext)
}

let highlighter: Promise<HighlighterCore> | null = null
const loaded = new Map<string, Promise<void>>()

function getHighlighter(): Promise<HighlighterCore> {
  highlighter ??= createHighlighterCore({
    themes: [import('@shikijs/themes/github-light'), import('@shikijs/themes/github-dark')],
    langs: [],
    engine: createJavaScriptRegexEngine(),
  })
  return highlighter
}

// highlight returns Shiki HTML for code, colored through CSS variables so
// light and dark themes switch without re-highlighting.
export async function highlight(code: string, lang: string): Promise<string> {
  const hl = await getHighlighter()
  let ready = loaded.get(lang)
  if (!ready) {
    const loader = loaders[lang]
    if (!loader) throw new Error(`no grammar for ${lang}`)
    ready = loader().then((m) => hl.loadLanguage(m.default))
    loaded.set(lang, ready)
  }
  await ready
  return hl.codeToHtml(code, { lang, themes: { light: 'github-light', dark: 'github-dark' }, defaultColor: false })
}
