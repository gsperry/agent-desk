import { describe, expect, it } from 'vitest'
import type { SystemStats } from '../types'
import { bytes, diskLevel, diskName, memoryLevel, statAlerts, statRows, type Level } from './stats'

const GB = 1024 ** 3

const stats = (p: Partial<SystemStats> = {}): SystemStats => ({
  supported: true,
  at: '2026-11-13T00:00:00Z',
  cpu: { load: [3, 2.75, 3.11], cores: 12 },
  memory: { total: 36 * GB, available: 7 * GB, pressure: 'normal' },
  swap: { total: 3 * GB, used: 1.7 * GB },
  disks: [{ mount: '/System/Volumes/Data', total: 460 * GB, free: 94 * GB }],
  ...p,
})

describe('bytes', () => {
  it.each([
    [94 * GB, '94 GB'],
    [1.74 * GB, '1.7 GB'],
    [0.05 * GB, '51 MB'],
    [2000 * GB, '2.0 TB'],
  ])('%d', (n, want) => {
    expect(bytes(n)).toBe(want)
  })
})

describe('levels', () => {
  it.each([
    ['plenty', 460, 94, 'ok'],
    ['large disk under 20 GB', 2000, 19, 'warn'],
    ['large disk under 5 GB', 2000, 4, 'critical'],
    ['small disk under 10%', 100, 9, 'warn'],
    ['small disk under 5%', 100, 4.9, 'critical'],
    ['unknown size', 0, 0, 'ok'],
  ] as const)('disk: %s', (_, total, free, want) => {
    expect(diskLevel(total * GB, free * GB)).toBe(want)
  })

  it.each([
    ['system says normal', { total: 36, available: 1, pressure: 'normal' }, 'ok'],
    ['system says warn', { total: 36, available: 20, pressure: 'warn' }, 'warn'],
    ['system says critical', { total: 36, available: 20, pressure: 'critical' }, 'critical'],
    ['no reading, plenty', { total: 16, available: 4 }, 'ok'],
    ['no reading, under 10%', { total: 16, available: 1.5 }, 'warn'],
    ['no reading, under 5%', { total: 16, available: 0.5 }, 'critical'],
  ] as const)('memory: %s', (_, m, want) => {
    expect(memoryLevel({ ...m, total: m.total * GB, available: m.available * GB })).toBe(want)
  })

  it('names disks by their last part', () => {
    expect(diskName('/System/Volumes/Data')).toBe('Data')
    expect(diskName('/')).toBe('/')
  })
})

describe('statRows', () => {
  it('describes a healthy machine', () => {
    const rows = statRows(stats())
    expect(rows.map((r) => [r.label, r.value, r.level])).toEqual([
      ['CPU', '25%', 'ok'],
      ['Memory', '29 GB / 36 GB', 'ok'],
      ['Swap', '1.7 GB', 'ok'],
      ['Disk', '94 GB free', 'ok'],
    ])
    expect(rows[0]?.detail).toBe('Load 3.00 2.75 3.11 over 1, 5 and 15 minutes, on 12 cores')
    expect(rows[3]?.fill).toBeCloseTo(366 / 460)
  })

  it('flags load, swap and disks, and names disks when there are several', () => {
    const rows = statRows(
      stats({
        cpu: { load: [30, 20, 10], cores: 12 },
        swap: { total: 24 * GB, used: 20 * GB },
        disks: [
          { mount: '/', total: 460 * GB, free: 94 * GB },
          { mount: '/Volumes/Scratch', total: 1000 * GB, free: 3 * GB },
        ],
      }),
    )
    expect(rows.map((r) => [r.label, r.level])).toEqual([
      ['CPU', 'critical'],
      ['Memory', 'ok'],
      ['Swap', 'critical'],
      ['Disk /', 'ok'],
      ['Disk Scratch', 'critical'],
    ])
    expect(rows[0]?.fill).toBe(1)
  })

  it('leaves out what was not read', () => {
    const rows = statRows(stats({ cpu: { load: [0, 0, 0], cores: 0 }, memory: { total: 0, available: 0 }, swap: { total: 0, used: 0 }, disks: [] }))
    expect(rows).toEqual([])
  })
})

describe('statAlerts', () => {
  it('alerts once when a disk or memory turns critical, and again after it recovers', () => {
    const levels = new Map<string, Level>()
    expect(statAlerts(stats(), levels)).toEqual([])
    const full = stats({ disks: [{ mount: '/System/Volumes/Data', total: 460 * GB, free: 3 * GB }], memory: { total: 36 * GB, available: 1 * GB, pressure: 'critical' } })
    const alerts = statAlerts(full, levels)
    expect(alerts.map((a) => a.tag)).toEqual(['system:memory', 'system:disk:/System/Volumes/Data'])
    expect(alerts[1]?.body).toBe('3.0 GB free on /System/Volumes/Data. Builds and agents may fail.')
    expect(statAlerts(full, levels)).toEqual([])
    statAlerts(stats(), levels)
    expect(statAlerts(full, levels)).toHaveLength(2)
  })

  it('does not alert on load or swap', () => {
    const levels = new Map<string, Level>()
    expect(statAlerts(stats({ cpu: { load: [99, 99, 99], cores: 4 }, swap: { total: 40 * GB, used: 30 * GB } }), levels)).toEqual([])
  })
})
