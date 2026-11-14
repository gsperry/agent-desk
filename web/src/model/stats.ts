import type { SystemStats } from '../types'

export type Level = 'ok' | 'warn' | 'critical'

// StatRow is one line of the sidebar's system stats.
export interface StatRow {
  key: string
  label: string
  value: string
  // fill is how full the bar is, 0 to 1.
  fill: number
  level: Level
  detail: string
}

const GB = 1024 ** 3

// bytes formats a size in GB, with one decimal under 10 GB, and in TB from
// 1000 GB.
export function bytes(n: number): string {
  const gb = n / GB
  if (gb >= 1000) return `${(gb / 1024).toFixed(1)} TB`
  if (gb >= 10) return `${String(Math.round(gb))} GB`
  if (gb >= 0.1) return `${gb.toFixed(1)} GB`
  return `${String(Math.round(n / 1024 ** 2))} MB`
}

const clamp = (n: number) => (Number.isFinite(n) ? Math.min(1, Math.max(0, n)) : 0)

// The thresholds. Load above the core count means work is queueing; twice
// the count is a machine that feels stuck.
export const limits = {
  loadWarn: 1,
  loadCritical: 2,
  // Without the system's own pressure reading: available memory share.
  memoryWarn: 0.1,
  memoryCritical: 0.05,
  // Swap in use as a share of physical memory.
  swapWarn: 0.25,
  swapCritical: 0.5,
  // Free disk: the smaller of a share of the disk and a fixed size, so a
  // large disk is not flagged with hundreds of GB free.
  diskWarn: { share: 0.1, bytes: 20 * GB },
  diskCritical: { share: 0.05, bytes: 5 * GB },
}

export function diskLevel(total: number, free: number): Level {
  if (total <= 0) return 'ok'
  if (free < Math.min(total * limits.diskCritical.share, limits.diskCritical.bytes)) return 'critical'
  if (free < Math.min(total * limits.diskWarn.share, limits.diskWarn.bytes)) return 'warn'
  return 'ok'
}

export function memoryLevel(m: SystemStats['memory']): Level {
  if (m.pressure === 'critical') return 'critical'
  if (m.pressure === 'warn') return 'warn'
  if (m.pressure === 'normal' || m.total <= 0) return 'ok'
  const share = m.available / m.total
  return share < limits.memoryCritical ? 'critical' : share < limits.memoryWarn ? 'warn' : 'ok'
}

// diskName is a mount point's short name: its last part, or / for the
// root.
export function diskName(mount: string): string {
  const parts = mount.split('/').filter(Boolean)
  return parts.at(-1) ?? '/'
}

export function statRows(st: SystemStats): StatRow[] {
  const rows: StatRow[] = []
  const { cpu, memory, swap, disks } = st
  if (cpu.cores > 0) {
    const ratio = cpu.load[0] / cpu.cores
    rows.push({
      key: 'cpu',
      label: 'CPU',
      value: `${String(Math.round(ratio * 100))}%`,
      fill: clamp(ratio),
      level: ratio > limits.loadCritical ? 'critical' : ratio > limits.loadWarn ? 'warn' : 'ok',
      detail: `Load ${cpu.load.map((l) => l.toFixed(2)).join(' ')} over 1, 5 and 15 minutes, on ${String(cpu.cores)} cores`,
    })
  }
  if (memory.total > 0) {
    const used = memory.total - memory.available
    rows.push({
      key: 'memory',
      label: 'Memory',
      value: `${bytes(used)} / ${bytes(memory.total)}`,
      fill: clamp(used / memory.total),
      level: memoryLevel(memory),
      detail: `${bytes(memory.available)} available without swapping${memory.pressure ? `; memory pressure ${memory.pressure}` : ''}`,
    })
  }
  if (swap.total > 0 || swap.used > 0) {
    const share = memory.total > 0 ? swap.used / memory.total : 0
    rows.push({
      key: 'swap',
      label: 'Swap',
      value: bytes(swap.used),
      fill: clamp(swap.total > 0 ? swap.used / swap.total : 0),
      level: share > limits.swapCritical ? 'critical' : share > limits.swapWarn ? 'warn' : 'ok',
      detail: `${bytes(swap.used)} of ${bytes(swap.total)} swap in use`,
    })
  }
  for (const d of disks) {
    rows.push({
      key: `disk:${d.mount}`,
      label: disks.length > 1 ? `Disk ${diskName(d.mount)}` : 'Disk',
      value: `${bytes(d.free)} free`,
      fill: clamp(d.total > 0 ? (d.total - d.free) / d.total : 0),
      level: diskLevel(d.total, d.free),
      detail: `${d.mount}: ${bytes(d.free)} free of ${bytes(d.total)}`,
    })
  }
  return rows
}

export interface StatAlert {
  // tag names the alert; a repeat replaces the earlier notification.
  tag: string
  title: string
  body: string
}

// statAlerts is the notifications due when a disk or memory turns
// critical. levels is what each row was at the last check, and comes back
// updated; a row alerts again only after it recovers.
export function statAlerts(st: SystemStats, levels: Map<string, Level>): StatAlert[] {
  const out: StatAlert[] = []
  for (const row of statRows(st)) {
    const was = levels.get(row.key)
    levels.set(row.key, row.level)
    if (row.level !== 'critical' || was === 'critical') continue
    if (row.key === 'memory') {
      out.push({ tag: 'system:memory', title: 'Memory is running out', body: `${row.value} in use. Agents and builds may slow down or fail.` })
    } else if (row.key.startsWith('disk:')) {
      const d = st.disks.find((x) => `disk:${x.mount}` === row.key)
      if (d) out.push({ tag: `system:${row.key}`, title: 'Disk almost full', body: `${bytes(d.free)} free on ${d.mount}. Builds and agents may fail.` })
    }
  }
  return out
}
