import type { ModelChoice, ModelOptions } from '../types'

// pick is the model and effort a dialog starts on: the model the session
// runs now, at its own default effort.
export function pick(options: ModelOptions): { model: string; effort: string } {
  const m = options.models.find((c) => c.current) ?? options.models[0]
  return m ? { model: m.label, effort: startEffort(m) } : { model: '', effort: '' }
}

// startEffort is the level a model opens at: its default, else the first
// it offers.
export function startEffort(m: ModelChoice): string {
  return m.defaultEffort ?? m.efforts[0] ?? ''
}

// effortFor keeps the chosen effort when a newly chosen model takes it, and
// falls back to that model's start otherwise.
export function effortFor(m: ModelChoice, effort: string): string {
  return m.efforts.includes(effort) ? effort : startEffort(m)
}
