import { beforeEach, expect, it, vi } from 'vitest'
import { Track, type RemoteAudioTrack, type Room } from 'livekit-client'
import { applyRoomMix, applyTrackVolume, loadMix, saveMix } from './playbackMix'
beforeEach(() => localStorage.clear())
it('changes only listener playback and keeps voice independent from stream audio', () => {
  saveMix({ voice: 100, stream: 20, ducking: false, reduction: 70 })
  const voice = { setVolume: vi.fn() } as unknown as RemoteAudioTrack
  const stream = { setVolume: vi.fn() } as unknown as RemoteAudioTrack
  const room = { remoteParticipants: new Map([['friend', { getTrackPublications: () => [
    { source: Track.Source.Microphone, audioTrack: voice },
    { source: Track.Source.ScreenShareAudio, audioTrack: stream },
  ] }]]) } as unknown as Room
  applyRoomMix(room)
  expect(voice.setVolume).toHaveBeenLastCalledWith(1)
  expect(stream.setVolume).toHaveBeenLastCalledWith(0.2)
  saveMix({ voice: 100, stream: 0, ducking: false, reduction: 70 })
  applyRoomMix(room)
  expect(voice.setVolume).toHaveBeenLastCalledWith(1)
  expect(stream.setVolume).toHaveBeenLastCalledWith(0)
  const reconnected = { setVolume: vi.fn() } as unknown as RemoteAudioTrack
  applyTrackVolume(reconnected, Track.Source.ScreenShareAudio)
  expect(reconnected.setVolume).toHaveBeenCalledWith(0)
})
it('restores saved values and normalizes invalid levels', () => {
  expect(loadMix()).toEqual({voice:100,stream:100,ducking:false,reduction:70})
  saveMix({voice:Infinity,stream:-10,ducking:false,reduction:70})
  expect(loadMix()).toEqual({voice:100,stream:0,ducking:false,reduction:70})
})

it('boosts to 200% and ducks only the speaking author’s computer audio when enabled', () => {
  const track = { setVolume: vi.fn() } as unknown as RemoteAudioTrack
  const mix = { voice: 200, stream: 150, ducking: true, reduction: 80 }
  applyTrackVolume(track, Track.Source.Microphone, mix, true)
  expect(track.setVolume).toHaveBeenLastCalledWith(2)
  applyTrackVolume(track, Track.Source.ScreenShareAudio, mix, true)
  expect(vi.mocked(track.setVolume).mock.lastCall?.[0]).toBeCloseTo(0.3)
  applyTrackVolume(track, Track.Source.ScreenShareAudio, mix, false)
  expect(track.setVolume).toHaveBeenLastCalledWith(1.5)
  applyTrackVolume(track, Track.Source.ScreenShareAudio, { ...mix, ducking: false }, true)
  expect(track.setVolume).toHaveBeenLastCalledWith(1.5)
})
it('ducks only the stream belonging to the person speaking', () => {
  saveMix({ voice: 100, stream: 100, ducking: true, reduction: 100 })
  const speakingStream = { setVolume: vi.fn() } as unknown as RemoteAudioTrack
  const silentStream = { setVolume: vi.fn() } as unknown as RemoteAudioTrack
  const participant = (track: RemoteAudioTrack, isSpeaking: boolean) => ({ isSpeaking, getTrackPublications: () => [{ source: Track.Source.ScreenShareAudio, audioTrack: track }] })
  const room = { remoteParticipants: new Map([['a', participant(speakingStream, true)], ['b', participant(silentStream, false)]]) } as unknown as Room
  applyRoomMix(room)
  expect(speakingStream.setVolume).toHaveBeenLastCalledWith(0)
  expect(silentStream.setVolume).toHaveBeenLastCalledWith(1)
})
