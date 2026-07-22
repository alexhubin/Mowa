import { Track, type AudioProcessorOptions, type LocalAudioTrack, type TrackProcessor } from 'livekit-client'
import { normalizeMicrophoneGain } from './deviceSettings'

class MicrophoneGainProcessor implements TrackProcessor<Track.Kind.Audio, AudioProcessorOptions> {
  readonly name = 'mova-microphone-gain'
  processedTrack?: MediaStreamTrack

  private audioContext?: AudioContext
  private source?: MediaStreamAudioSourceNode
  private gainNode?: GainNode
  private destination?: MediaStreamAudioDestinationNode
  private gain = 1

  constructor(percent: number) {
    this.gain = normalizeMicrophoneGain(percent) / 100
  }

  async init(options: AudioProcessorOptions) {
    this.audioContext = options.audioContext
    this.connect(options.track)
  }

  async restart(options: AudioProcessorOptions) {
    this.disconnect()
    this.connect(options.track)
  }

  async destroy() {
    this.disconnect()
    this.audioContext = undefined
  }

  setGain(percent: number) {
    this.gain = normalizeMicrophoneGain(percent) / 100
    if (!this.gainNode || !this.audioContext) return
    this.gainNode.gain.setTargetAtTime(this.gain, this.audioContext.currentTime, 0.015)
  }

  private connect(track: MediaStreamTrack) {
    if (!this.audioContext) throw new Error('AudioContext недоступен')

    this.source = this.audioContext.createMediaStreamSource(new MediaStream([track]))
    this.gainNode = this.audioContext.createGain()
    this.destination = this.audioContext.createMediaStreamDestination()
    this.gainNode.gain.value = this.gain
    this.source.connect(this.gainNode)
    this.gainNode.connect(this.destination)
    this.processedTrack = this.destination.stream.getAudioTracks()[0]
  }

  private disconnect() {
    this.source?.disconnect()
    this.gainNode?.disconnect()
    this.processedTrack?.stop()
    this.source = undefined
    this.gainNode = undefined
    this.destination = undefined
    this.processedTrack = undefined
  }
}

const gainProcessors = new WeakMap<LocalAudioTrack, MicrophoneGainProcessor>()

export async function applyMicrophoneGain(track: LocalAudioTrack | undefined, percent: number) {
  if (!track) return

  const existing = gainProcessors.get(track)
  if (existing) {
    existing.setGain(percent)
    return
  }

  const processor = new MicrophoneGainProcessor(percent)
  gainProcessors.set(track, processor)
  try {
    await track.setProcessor(processor)
  } catch (error) {
    gainProcessors.delete(track)
    throw error
  }
}
