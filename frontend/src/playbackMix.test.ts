import { beforeEach, expect, it, vi } from 'vitest'
import { Track, type RemoteAudioTrack, type Room } from 'livekit-client'
import { applyRoomMix, applyTrackVolume, loadMix, saveMix } from './playbackMix'
beforeEach(() => localStorage.clear())
it('changes only listener playback and keeps voice independent from stream audio', () => {
  saveMix({ voice: 100, stream: 20 })
  const voice = { setVolume: vi.fn() } as unknown as RemoteAudioTrack
  const stream = { setVolume: vi.fn() } as unknown as RemoteAudioTrack
  const room = { remoteParticipants: new Map([['friend', { getTrackPublications: () => [
    { source: Track.Source.Microphone, audioTrack: voice },
    { source: Track.Source.ScreenShareAudio, audioTrack: stream },
  ] }]]) } as unknown as Room
  applyRoomMix(room)
  expect(voice.setVolume).toHaveBeenLastCalledWith(1)
  expect(stream.setVolume).toHaveBeenLastCalledWith(0.2)
  saveMix({ voice: 100, stream: 0 })
  applyRoomMix(room)
  expect(voice.setVolume).toHaveBeenLastCalledWith(1)
  expect(stream.setVolume).toHaveBeenLastCalledWith(0)
  const reconnected = { setVolume: vi.fn() } as unknown as RemoteAudioTrack
  applyTrackVolume(reconnected, Track.Source.ScreenShareAudio)
  expect(reconnected.setVolume).toHaveBeenCalledWith(0)
})
it('restores saved values and normalizes invalid levels', () => {
  expect(loadMix()).toEqual({voice:100,stream:100})
  saveMix({voice:Infinity,stream:-10})
  expect(loadMix()).toEqual({voice:100,stream:0})
})
