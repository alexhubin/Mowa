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
  attributes: Record<string, string> = {}
  setAttributes = vi.fn(async (attrs: Record<string, string>) => { Object.assign(this.attributes, attrs) })
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
  createScreenTracks = vi.fn(async () => [{
    kind: Track.Kind.Video,
    mediaStreamTrack: { readyState: 'live' },
    stop: vi.fn(), attach: vi.fn(), detach: vi.fn(), on: vi.fn(), off: vi.fn(),
  }])
  publishTrack = vi.fn(async (track: Awaited<ReturnType<FakeParticipant['createScreenTracks']>>[number]) => {
    this.isScreenShareEnabled = true
    this.publications.set(Track.Source.ScreenShare, { isMuted: false, track })
    this.room.emit(RoomEvent.LocalTrackPublished)
  })
  unpublishTrack = vi.fn(async () => {
    this.isScreenShareEnabled = false
    this.publications.delete(Track.Source.ScreenShare)
    this.room.emit(RoomEvent.LocalTrackUnpublished)
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
  off(event: string, listener: () => void) {
    this.handlers.set(event, (this.handlers.get(event) ?? []).filter(fn => fn !== listener))
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
  supportsAV1: () => true,
  supportsVP9: () => true,
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

let anonymous = false
let guestSession: { id: string; display_name: string } | null = null
let guestJoinError = false
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
  const caps = { getCapabilities: () => ({ codecs: [{ mimeType: 'video/H264', sdpFmtpLine: 'profile-level-id=42e01f;packetization-mode=1' }] }) }
  vi.stubGlobal('RTCRtpSender', caps)
  vi.stubGlobal('RTCRtpReceiver', caps)
  anonymous = false
  guestSession = null
  guestJoinError = false
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
      if (path === '/api/auth/me') return anonymous ? new Response('{}', { status: 401 }) : json(user)
      if (path.endsWith('/guest')) {
        if (init?.method === 'POST') {
          if (guestJoinError) return new Response(JSON.stringify({ error: 'Комната уже закрыта' }), { status: 404 })
          guestSession = { id: 'guest_me', display_name: JSON.parse(String(init.body)).display_name }
          return json(guestSession)
        }
        if (init?.method === 'DELETE') {
          guestSession = null
          return new Response(null, { status: 204 })
        }
        return guestSession ? json(guestSession) : new Response('{}', { status: 401 })
      }
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
    await screen.findByRole('button', { name: 'Join call' }),
  )
  await screen.findByRole('button', { name: 'Mute microphone' })
  expect(document.exitFullscreen).not.toHaveBeenCalled()
  return media.rooms[0]
}

describe('Svelte LiveKit lifecycle', () => {
  it('reacts to mutable participant and screen-track events and releases media on navigation', async () => {
    const room = await join()
    await fireEvent.click(
      screen.getByRole('button', { name: 'Mute microphone' }),
    )
    await screen.findByRole('button', { name: 'Unmute microphone' })
    room.localParticipant.isSpeaking = true
    room.emit(RoomEvent.ActiveSpeakersChanged)
    await screen.findByText('speaking')
    await fireEvent.click(
      screen.getByRole('button', { name: 'Share screen' }),
    )
    await screen.findByRole('button', { name: 'Stop sharing' })
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
    await screen.findByRole('heading', { name: 'Friends' })
    expect(track.stop).toHaveBeenCalled()
    expect(track.detach).toHaveBeenCalledOnce()
    expect(room.disconnect).toHaveBeenCalledOnce()
    expect(source.close).toHaveBeenCalledOnce()
  })

  it('republishes one capture as compatible peers join, update capabilities and leave', async () => {
    const caps = { getCapabilities: () => ({ codecs: [
      { mimeType: 'video/AV1' }, { mimeType: 'video/VP9' },
      { mimeType: 'video/H264', sdpFmtpLine: 'profile-level-id=42e01f;packetization-mode=1' },
    ] }) }
    vi.stubGlobal('RTCRtpSender', caps)
    vi.stubGlobal('RTCRtpReceiver', caps)
    const room = await join()
    await fireEvent.click(screen.getByRole('button', { name: 'Share screen' }))
    const published = (codec: string, maxBitrate: number) => waitFor(() =>
      expect(room.localParticipant.publishTrack).toHaveBeenLastCalledWith(expect.anything(), expect.objectContaining({
        videoCodec: codec, simulcast: false, backupCodec: false,
        screenShareEncoding: { maxBitrate, maxFramerate: 60 },
      })),
    )
    await published('av1', 8e6)
    const remote = new FakeParticipant(room)
    remote.identity = 'friend'
    remote.attributes = { 'mowa.video.receive.v1': 'vp9,h264' }
    room.remoteParticipants.set('friend', remote)
    room.emit(RoomEvent.ParticipantConnected)
    await published('vp9', 10e6)
    remote.attributes['mowa.video.receive.v1'] = 'h264'
    room.emit(RoomEvent.ParticipantAttributesChanged)
    await published('h264', 14e6)
    room.remoteParticipants.delete('friend')
    room.emit(RoomEvent.ParticipantDisconnected)
    await published('av1', 8e6)
    expect(room.localParticipant.createScreenTracks).toHaveBeenCalledOnce()
    expect(room.localParticipant.unpublishTrack).toHaveBeenCalledTimes(3)
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
        name: 'Another participant is already sharing',
      }),
    ).toBeDisabled()
    await fireEvent.click(screen.getByRole('button', { name: 'Open chat' }))
    const composer = screen.getByRole('textbox', { name: 'Message' })
    await fireEvent.input(composer, {
      target: { value: 'Message в комнате' },
    })
    await fireEvent.submit(composer.closest('form')!)
    await screen.findByText('Message в комнате')
    expect(composer).toHaveValue('')
    remote.publications.clear()
    room.emit(RoomEvent.TrackUnpublished)
    expect(
      await screen.findByRole('button', { name: 'Share screen' }),
    ).toBeEnabled()
  })

  it('does not connect after leaving while a token request is pending', async () => {
    let resolveToken!: (response: Response) => void
    tokenResponse = new Promise((resolve) => {
      resolveToken = resolve
    })
    render(App)
    await fireEvent.click(
      await screen.findByRole('button', { name: 'Join call' }),
    )
    navigate({ to: '/' })
    await screen.findByRole('heading', { name: 'Friends' })
    resolveToken(json({ token: 'late-token', server_url: 'wss://example.com' }))
    await tokenResponse
    await waitFor(() => expect(media.rooms).toHaveLength(0))
  })
})


describe('Account invitation', () => {
  it('requires login even when an old guest cookie exists', async () => {
    anonymous = true
    guestSession = { id: 'guest_me', display_name: 'Соня' }
    render(App)
    await screen.findByRole('heading', { name: 'Sign in' })
    expect(route.pathname).toBe('/login')
    expect(new URLSearchParams(location.search).get('next')).toBe('/r/MOWA-TEST')
    expect(media.rooms).toHaveLength(0)
    expect(vi.mocked(fetch).mock.calls.some(([path]) => String(path).endsWith('/guest') || String(path).endsWith('/token'))).toBe(false)
  })
})
