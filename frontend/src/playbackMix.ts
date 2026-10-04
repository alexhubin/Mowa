import { Track, type RemoteAudioTrack, type Room } from 'livekit-client'
export type PlaybackMix = { voice: number; stream: number }
const key = 'mowa.playback-mix.v1'
export const mixEvent = 'mowa:playback-mix'
export function percent(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? Math.max(0, Math.min(100, Math.round(value))) : 100
}
export function loadMix(): PlaybackMix {
  try {
    const value = JSON.parse(localStorage.getItem(key) ?? '')
    return { voice: percent(value.voice), stream: percent(value.stream) }
  } catch { return { voice: 100, stream: 100 } }
}
export function saveMix(mix: PlaybackMix) {
  localStorage.setItem(key, JSON.stringify({ voice: percent(mix.voice), stream: percent(mix.stream) }))
  window.dispatchEvent(new Event(mixEvent))
}
export function applyTrackVolume(track: RemoteAudioTrack, source: Track.Source, mix = loadMix()) {
  track.setVolume(percent(source === Track.Source.ScreenShareAudio ? mix.stream : mix.voice) / 100)
}
export function applyRoomMix(room: Room) {
  const mix = loadMix()
  for (const participant of room.remoteParticipants.values()) {
    for (const publication of participant.getTrackPublications()) {
      const track = publication.audioTrack
      if (track && 'setVolume' in track) applyTrackVolume(track, publication.source, mix)
    }
  }
}
