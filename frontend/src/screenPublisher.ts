import { Room, RoomEvent, Track, TrackEvent, type LocalVideoTrack } from 'livekit-client'
import { RECEIVE_CODECS, bitrate, browserCodecs, chooseCodec, parseCodecs, type ScreenCodec } from './videoPolicy'

// Exactly one encoding. Republish the existing capture when the room's common codec changes.
export class ScreenPublisher {
  private caps = browserCodecs()
  private track?: LocalVideoTrack
  private codec?: ScreenCodec
  private high = true
  private closed = false
  private queue: Promise<unknown> = Promise.resolve()
  private generation = 0
  constructor(private room: Room, private error: (message: string) => void) {
    room.on(RoomEvent.ParticipantConnected, this.changed)
    room.on(RoomEvent.ParticipantDisconnected, this.changed)
    room.on(RoomEvent.ParticipantAttributesChanged, this.changed)
    room.on(RoomEvent.Reconnected, this.reconnected)
  }
  async announce() {
    await this.room.localParticipant.setAttributes({ [RECEIVE_CODECS]: this.caps.receive.join(',') })
  }
  private reconnected = () => { void this.announce().then(this.changed).catch(this.report) }
  private report = (error: unknown) => {
    if (!this.closed) this.error(error instanceof Error ? error.message : 'Could not negotiate video codec')
  }
  private selected() {
    return chooseCodec(this.caps.send, Array.from(this.room.remoteParticipants.values(), p => parseCodecs(p.attributes[RECEIVE_CODECS])))
  }
  private enqueue<T>(action: () => Promise<T>): Promise<T> {
    const next = this.queue.then(action)
    this.queue = next.catch(() => undefined)
    return next
  }
  private changed = () => { void this.enqueue(() => this.reconcile()).catch(this.report) }
  private ended = () => { void this.stop().catch(this.report) }
  async start(high: boolean) {
    if (this.track) throw new Error('Screen sharing is already active')
    if (!this.selected()) throw new Error('No common video codec with the participants in this call.')
    const generation = ++this.generation
    // Call the picker directly from the user gesture, before queuing network operations.
    const tracks = await this.room.localParticipant.createScreenTracks({
      audio: false, contentHint: 'detail',
      resolution: { width: high ? 1920 : 1280, height: high ? 1080 : 720, frameRate: 60 },
    })
    await this.enqueue(async () => {
      if (this.closed || generation !== this.generation) { tracks.forEach(t => t.stop()); return }
      this.track = tracks.find(t => t.kind === Track.Kind.Video) as LocalVideoTrack | undefined
      this.high = high
      this.track?.on(TrackEvent.Ended, this.ended)
      try { await this.reconcile() } catch (error) { await this.clear(); throw error }
    })
  }
  private async reconcile() {
    const track = this.track
    if (this.closed || !track || track.mediaStreamTrack.readyState === 'ended') return
    const codec = this.selected()
    if (codec === this.codec && codec) return
    if (this.codec) {
      await this.room.localParticipant.unpublishTrack(track, false)
      this.codec = undefined
    }
    if (!codec) {
      await this.clear()
      throw new Error('Screen sharing stopped: no common video codec with all participants.')
    }
    if (this.closed) return
    try {
      await this.room.localParticipant.publishTrack(track, {
        source: Track.Source.ScreenShare, videoCodec: codec,
        screenShareEncoding: { maxBitrate: bitrate(codec, this.high), maxFramerate: 60 },
        simulcast: false, backupCodec: false,
        ...(codec !== 'h264' ? { scalabilityMode: 'L1T1' as const } : {}),
        degradationPreference: 'maintain-resolution',
      })
      this.codec = codec
    } catch (error) {
      await this.clear()
      throw error
    }
  }
  private async clear() {
    const track = this.track
    this.track = undefined
    this.codec = undefined
    if (track) {
      track.off(TrackEvent.Ended, this.ended)
      track.stop()
      await this.room.localParticipant.unpublishTrack(track)
    }
  }
  stop() {
    ++this.generation
    return this.enqueue(() => this.clear())
  }
  dispose() {
    this.closed = true
    ++this.generation
    this.room.off(RoomEvent.ParticipantConnected, this.changed)
    this.room.off(RoomEvent.ParticipantDisconnected, this.changed)
    this.room.off(RoomEvent.ParticipantAttributesChanged, this.changed)
    this.room.off(RoomEvent.Reconnected, this.reconnected)
    this.track?.stop()
    void this.stop().catch(() => undefined)
  }
}
