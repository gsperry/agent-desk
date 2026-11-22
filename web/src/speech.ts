import { createContext, useContext } from 'react'

// SpeakFetch asks the server to read text aloud, through the
// text-to-speech service the owner configured, and returns MP3 audio.
export type SpeakFetch = (text: string) => Promise<{ audio?: Blob; error?: string }>

// SpeakContext carries SpeakFetch while read aloud is available here;
// null hides every Read aloud button.
export const SpeakContext = createContext<SpeakFetch | null>(null)

export function useSpeak(): SpeakFetch | null {
  return useContext(SpeakContext)
}

export interface PlayerState {
  // key names the reply being read, or null.
  key: string | null
  phase: 'loading' | 'playing' | null
  error: string | null
}

// silentWav is a short silent WAV. Playing it in the click that asks for
// audio unlocks playback, which WebKit otherwise refuses once the audio
// arrives after a network wait.
function silentWav(): Blob {
  const samples = 800
  const buf = new ArrayBuffer(44 + samples)
  const v = new DataView(buf)
  const str = (o: number, s: string) => {
    for (let i = 0; i < s.length; i++) v.setUint8(o + i, s.charCodeAt(i))
  }
  str(0, 'RIFF')
  v.setUint32(4, 36 + samples, true)
  str(8, 'WAVE')
  str(12, 'fmt ')
  v.setUint32(16, 16, true)
  v.setUint16(20, 1, true)
  v.setUint16(22, 1, true)
  v.setUint32(24, 8000, true)
  v.setUint32(28, 8000, true)
  v.setUint16(32, 1, true)
  v.setUint16(34, 8, true)
  str(36, 'data')
  v.setUint32(40, samples, true)
  for (let i = 0; i < samples; i++) v.setUint8(44 + i, 128)
  return new Blob([buf], { type: 'audio/wav' })
}

type Listener = () => void

// Player reads one reply at a time: starting another stops the first.
export class Player {
  private state: PlayerState = { key: null, phase: null, error: null }
  private listeners = new Set<Listener>()
  private audio: HTMLAudioElement | null = null
  private url: string | null = null
  // run counts plays, so a late answer for a stopped play is dropped.
  private run = 0

  get = (): PlayerState => this.state

  subscribe = (l: Listener) => {
    this.listeners.add(l)
    return () => {
      this.listeners.delete(l)
    }
  }

  private set(next: PlayerState) {
    this.state = next
    for (const l of this.listeners) l()
  }

  // play must be called from the click that asks for it.
  play(key: string, text: string, fetchAudio: SpeakFetch) {
    this.stop()
    const run = ++this.run
    const audio = new Audio()
    this.audio = audio
    const unlock = URL.createObjectURL(silentWav())
    audio.src = unlock
    void audio.play().catch(() => undefined)
    this.set({ key, phase: 'loading', error: null })
    void fetchAudio(text).then((r) => {
      URL.revokeObjectURL(unlock)
      if (run !== this.run) return
      if (!r.audio) {
        this.set({ key, phase: null, error: r.error ?? 'Could not read this reply aloud.' })
        return
      }
      this.url = URL.createObjectURL(r.audio)
      audio.src = this.url
      audio.onended = () => {
        if (run === this.run) this.stop()
      }
      audio.play().then(
        () => {
          if (run === this.run) this.set({ key, phase: 'playing', error: null })
        },
        (e: unknown) => {
          if (run === this.run) this.set({ key, phase: null, error: `The audio could not play: ${String(e)}` })
        },
      )
    })
  }

  stop() {
    this.run++
    if (this.audio) {
      this.audio.pause()
      this.audio.removeAttribute('src')
      this.audio = null
    }
    if (this.url) {
      URL.revokeObjectURL(this.url)
      this.url = null
    }
    if (this.state.key !== null) this.set({ key: null, phase: null, error: null })
  }
}

export const player = new Player()
