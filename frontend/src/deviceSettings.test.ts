import { beforeEach, describe, expect, it } from 'vitest'
import { loadDeviceSettings, normalizeMicrophoneGain, saveDeviceSettings } from './deviceSettings'

describe('device settings', () => {
  beforeEach(() => localStorage.clear())

  it('uses safe audio defaults for a new browser', () => {
    expect(loadDeviceSettings()).toEqual({
      audioInputId: '',
      audioOutputId: '',
      microphoneGain: 100,
      noiseSuppression: true,
    })
  })

  it('keeps existing v1 device selections when adding processing settings', () => {
    localStorage.setItem('mova.audio-devices.v1', JSON.stringify({ audioInputId: 'mic-1', audioOutputId: 'speaker-1' }))
    expect(loadDeviceSettings()).toEqual({
      audioInputId: 'mic-1',
      audioOutputId: 'speaker-1',
      microphoneGain: 100,
      noiseSuppression: true,
    })
  })

  it('clamps microphone gain to the supported range', () => {
    expect(normalizeMicrophoneGain(-25)).toBe(0)
    expect(normalizeMicrophoneGain(126.4)).toBe(126)
    expect(normalizeMicrophoneGain(900)).toBe(200)
    expect(normalizeMicrophoneGain('loud')).toBe(100)
  })

  it('saves and restores processing settings', () => {
    saveDeviceSettings({ audioInputId: 'mic-2', audioOutputId: '', microphoneGain: 145, noiseSuppression: false })
    expect(loadDeviceSettings()).toEqual({ audioInputId: 'mic-2', audioOutputId: '', microphoneGain: 145, noiseSuppression: false })
  })
})
