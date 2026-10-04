import { Track, type RemoteAudioTrack, type Room } from 'livekit-client'
export type PlaybackMix = { voice: number; stream: number; ducking: boolean; reduction: number }
const key = 'mowa.playback-mix.v1'
export const mixEvent = 'mowa:playback-mix'
export function percent(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? Math.max(0, Math.min(200, Math.round(value))) : 100
}
export function loadMix(): PlaybackMix {
  try {
    const value = JSON.parse(localStorage.getItem(key) ?? '')
    return { voice: percent(value.voice), stream: percent(value.stream), ducking: value.ducking === true, reduction: reduction(value.reduction) }
  } catch { return { voice: 100, stream: 100, ducking: false, reduction: 70 } }
}
function reduction(value: unknown) { return value === undefined ? 70 : Math.min(100, percent(value)) }
export function saveMix(mix: PlaybackMix) {
  localStorage.setItem(key, JSON.stringify({ voice: percent(mix.voice), stream: percent(mix.stream), ducking: mix.ducking === true, reduction: reduction(mix.reduction) }))
  window.dispatchEvent(new Event(mixEvent))
}
export function applyTrackVolume(track: RemoteAudioTrack, source: Track.Source, mix = loadMix(), speaking = false) {
  const stream = source === Track.Source.ScreenShareAudio
  track.setVolume(percent(stream ? mix.stream : mix.voice) / 100 * (stream && mix.ducking && speaking ? 1 - reduction(mix.reduction) / 100 : 1))
}
export function applyRoomMix(room: Room) {
  const mix = loadMix()
  for (const participant of room.remoteParticipants.values()) {
    for (const publication of participant.getTrackPublications()) {
      const track = publication.audioTrack
      if (track && 'setVolume' in track) applyTrackVolume(track, publication.source, mix, participant.isSpeaking)
    }
  }
}
