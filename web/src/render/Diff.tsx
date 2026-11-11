import type { DiffRow } from '../model/diff'

export function Diff({ rows }: { rows: DiffRow[] }) {
  if (rows.length === 0) return <p className="muted">No changes recorded.</p>
  return (
    <div className="diff">
      <table>
        <tbody>
          {rows.map((r, i) =>
            r.type === 'gap' ? (
              <tr key={i} className="diff-gap">
                <td colSpan={4}>⋯</td>
              </tr>
            ) : (
              <tr key={i} className={`diff-${r.type}`}>
                <td className="diff-no">{r.oldNo ?? ''}</td>
                <td className="diff-no">{r.newNo ?? ''}</td>
                <td className="diff-sign">{r.type === 'add' ? '+' : r.type === 'del' ? '−' : ' '}</td>
                <td className="diff-text">{r.text}</td>
              </tr>
            ),
          )}
        </tbody>
      </table>
    </div>
  )
}
