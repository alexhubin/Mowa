const storageKey = 'mova.audio-devices.v1'

export type LocalDeviceSettings = {
  audioInputId: string
  audioOutputId: string
  microphoneGain: number
  noiseSuppression: boolean
}

export const defaultDeviceSettings: LocalDeviceSettings = {
  audioInputId: '',
  audioOutputId: '',
  microphoneGain: 100,
  noiseSuppression: true,
}

export function normalizeMicrophoneGain(value: unknown) {
  if (typeof value !== 'number' || !Number.isFinite(value)) return defaultDeviceSettings.microphoneGain
  return Math.min(200, Math.max(0, Math.round(value)))
}

export function loadDeviceSettings(): LocalDeviceSettings {
  try {
    const parsed = JSON.parse(localStorage.getItem(storageKey) ?? '') as Partial<LocalDeviceSettings>
    return {
      audioInputId: typeof parsed.audioInputId === 'string' ? parsed.audioInputId : '',
      audioOutputId: typeof parsed.audioOutputId === 'string' ? parsed.audioOutputId : '',
      microphoneGain: normalizeMicrophoneGain(parsed.microphoneGain),
      noiseSuppression: typeof parsed.noiseSuppression === 'boolean' ? parsed.noiseSuppression : true,
    }
  } catch {
    return { ...defaultDeviceSettings }
  }
}

export function saveDeviceSettings(settings: LocalDeviceSettings) {
  localStorage.setItem(storageKey, JSON.stringify({
    ...settings,
    microphoneGain: normalizeMicrophoneGain(settings.microphoneGain),
  }))
}

export async function requestAndListAudioDevices() {
  const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
  stream.getTracks().forEach((track) => track.stop())
  const devices = await navigator.mediaDevices.enumerateDevices()
  return {
    inputs: devices.filter((device) => device.kind === 'audioinput'),
    outputs: devices.filter((device) => device.kind === 'audiooutput'),
  }
}
