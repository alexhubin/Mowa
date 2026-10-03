import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/svelte'
import { RoomEvent, Track } from 'livekit-client'
import App from './App.svelte'
import { navigate, route } from './navigation.svelte'

const media = vi.hoisted(() => ({ rooms: [] as FakeRoom[] }))

class FakeParticipant {
  identity = 'me'
  name = 'Алекс'
  isSpeaking = false
  isMicrophoneEnabled = false
  isScreenShareEnabled = false
  publications = new Map<
    string,
    {
      isMuted: boolean
      track?: {
        stop: () => void
        attach: (element: HTMLVideoElement) => void
        detach: (element: HTMLVideoElement) => void
      }
    }
  >()
  constructor(private room: FakeRoom) {}
  getTrackPublication(source: string) {
    return this.publications.get(source)
  }
  getTrackPublications() {
    return [...this.publications.values()]
  }
  setMicrophoneEnabled = vi.fn(async (enabled: boolean) => {
    this.isMicrophoneEnabled = enabled
    const publication = {
      isMuted: !enabled,
      track: { stop: vi.fn(), attach: vi.fn(), detach: vi.fn() },
    }
    this.publications.set(Track.Source.Microphone, publication)
    this.room.emit(RoomEvent.LocalTrackPublished)
    return publication
  })
  setScreenShareEnabled = vi.fn(async (enabled: boolean) => {
    this.isScreenShareEnabled = enabled
    if (enabled)
      this.publications.set(Track.Source.ScreenShare, {
        isMuted: false,
        track: { stop: vi.fn(), attach: vi.fn(), detach: vi.fn() },
      })
    else this.publications.delete(Track.Source.ScreenShare)
    this.room.emit(RoomEvent.LocalTrackPublished)
  })
}

class FakeRoom {
  state = 'disconnected'
  localParticipant = new FakeParticipant(this)
  remoteParticipants = new Map<string, FakeParticipant>()
  handlers = new Map<string, (() => void)[]>()
  constructor() {
    media.rooms.push(this)
  }
  on(event: string, listener: () => void) {
    this.handlers.set(event, [...(this.handlers.get(event) ?? []), listener])
    return this
  }
  emit(event: string) {
    this.handlers.get(event)?.forEach((listener) => listener())
  }
  connect = vi.fn(async () => {
    this.state = 'connected'
    this.emit(RoomEvent.ConnectionStateChanged)
  })
  disconnect = vi.fn(async () => {
    this.state = 'disconnected'
    this.emit(RoomEvent.ConnectionStateChanged)
  })
  startAudio = vi.fn(async () => undefined)
  switchActiveDevice = vi.fn(async () => undefined)
}

vi.mock('livekit-client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('livekit-client')>()),
  Room: class {
    constructor() {
      return new FakeRoom()
    }
  },
}))

class MockEventSource extends EventTarget {
  static instances: MockEventSource[] = []
  close = vi.fn()
  constructor(public url: string) {
    super()
    MockEventSource.instances.push(this)
  }
}

let tokenResponse: Promise<Response> | undefined
const user = {
  id: 'me',
  username: 'alex',
  display_name: 'Алекс',
  email: 'alex@example.com',
  must_change_password: false,
}
const json = (body: unknown) =>
  new Response(JSON.stringify(body), {
    headers: { 'Content-Type': 'application/json' },
  })

beforeEach(() => {
  media.rooms = []
  tokenResponse = undefined
  MockEventSource.instances = []
  history.replaceState(null, '', '/r/MOWA-TEST')
  route.pathname = '/r/MOWA-TEST'
  localStorage.clear()
  Element.prototype.scrollIntoView = vi.fn()
  Object.defineProperty(document, "fullscreenElement", { configurable: true, value: null })
  document.exitFullscreen = vi.fn(async () => undefined)
  vi.stubGlobal('EventSource', MockEventSource)
  vi.stubGlobal('matchMedia', () => ({ matches: false }))
  vi.stubGlobal(
    'fetch',
    vi.fn(async (path: string, init?: RequestInit) => {
      if (path === '/api/auth/me') return json(user)
      if (path === '/api/calls') return json([])
      if (path === '/api/account/settings')
        return json({ video_quality: 'high' })
      if (path === '/api/presence') return new Response(null, { status: 204 })
      if (path === '/api/friends')
        return json({ friends: [], incoming: [], outgoing: [] })
      if (path.endsWith('/token'))
        return (
          tokenResponse ??
          json({
            token: 'token',
            server_url: 'wss://example.com',
            expires_in: 600,
          })
        )
      if (path.endsWith('/messages'))
        return init?.method === 'POST'
          ? json({
              id: 'message',
              body: JSON.parse(String(init.body)).body,
              author: user,
              created_at: new Date().toISOString(),
            })
          : json([])
      if (path.startsWith('/api/rooms/'))
        return json({
          id: 'room',
          invite_code: 'MOWA-TEST',
          name: 'Комната',
          kind: 'group',
          owner_id: 'me',
        })
      throw new Error(`Unexpected request ${path}`)
    }),
  )
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

async function join() {
  render(App)
  await fireEvent.click(
    await screen.findByRole('button', { name: 'Войти в разговор' }),
  )
  await screen.findByRole('button', { name: 'Выключить микрофон' })
  expect(document.exitFullscreen).not.toHaveBeenCalled()
  return media.rooms[0]
}

describe('Svelte LiveKit lifecycle', () => {
  it('reacts to mutable participant and screen-track events and releases media on navigation', async () => {
    const room = await join()
    await fireEvent.click(
      screen.getByRole('button', { name: 'Выключить микрофон' }),
    )
    await screen.findByRole('button', { name: 'Включить микрофон' })
    room.localParticipant.isSpeaking = true
    room.emit(RoomEvent.ActiveSpeakersChanged)
    await screen.findByText('говорит')
    await fireEvent.click(
      screen.getByRole('button', { name: 'Показать экран' }),
    )
    await screen.findByRole('button', { name: 'Остановить показ экрана' })
    const track = room.localParticipant.getTrackPublication(
      Track.Source.ScreenShare,
    )!.track!
    await waitFor(() => expect(track.attach).toHaveBeenCalledOnce())
    room.emit(RoomEvent.ActiveSpeakersChanged)
    expect(track.attach).toHaveBeenCalledOnce()
    const source = MockEventSource.instances.find((source) =>
      source.url.endsWith('/messages/events'),
    )!
    navigate({ to: '/' })
    await screen.findByRole('heading', { name: 'Друзья' })
    expect(track.stop).toHaveBeenCalledOnce()
    expect(track.detach).toHaveBeenCalledOnce()
    expect(room.disconnect).toHaveBeenCalledOnce()
    expect(source.close).toHaveBeenCalledOnce()
  })

  it('blocks screen sharing while a remote participant shares and sends room messages', async () => {
    const room = await join()
    const remote = new FakeParticipant(room)
    remote.identity = 'friend'
    remote.name = 'Соня'
    remote.publications.set(Track.Source.ScreenShare, {
      isMuted: false,
      track: { stop: vi.fn(), attach: vi.fn(), detach: vi.fn() },
    })
    room.remoteParticipants.set(remote.identity, remote)
    room.emit(RoomEvent.ParticipantConnected)
    expect(
      await screen.findByRole('button', {
        name: 'Другой участник уже демонстрирует экран',
      }),
    ).toBeDisabled()
    await fireEvent.click(screen.getByRole('button', { name: 'Открыть чат' }))
    const composer = screen.getByRole('textbox', { name: 'Сообщение' })
    await fireEvent.input(composer, {
      target: { value: 'Сообщение в комнате' },
    })
    await fireEvent.submit(composer.closest('form')!)
    await screen.findByText('Сообщение в комнате')
    expect(composer).toHaveValue('')
    remote.publications.clear()
    room.emit(RoomEvent.TrackUnpublished)
    expect(
      await screen.findByRole('button', { name: 'Показать экран' }),
    ).toBeEnabled()
  })

  it('does not connect after leaving while a token request is pending', async () => {
    let resolveToken!: (response: Response) => void
    tokenResponse = new Promise((resolve) => {
      resolveToken = resolve
    })
    render(App)
    await fireEvent.click(
      await screen.findByRole('button', { name: 'Войти в разговор' }),
    )
    navigate({ to: '/' })
    await screen.findByRole('heading', { name: 'Друзья' })
    resolveToken(json({ token: 'late-token', server_url: 'wss://example.com' }))
    await tokenResponse
    await waitFor(() => expect(media.rooms).toHaveLength(0))
  })
})
