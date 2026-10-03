<script lang="ts">
  import { createMutation } from '@tanstack/svelte-query'

  import { SlidersHorizontal, X } from '@lucide/svelte'
  import { Room, Track } from 'livekit-client'
  import { api, type AccountSettings } from '../api'
  import {
    loadDeviceSettings,
    requestAndListAudioDevices,
    saveDeviceSettings,
    type LocalDeviceSettings,
  } from '../deviceSettings'
  import { applyMicrophoneGain } from '../microphoneProcessor'

  import CallDeviceSelect from './CallDeviceSelect.svelte'

  let {
    room,
    settings,
    onClose,
    onSettingsSaved,
  }: {
    room: Room | null
    settings?: AccountSettings
    onClose: () => void
    onSettingsSaved: (settings: AccountSettings) => void
  } = $props()
  let deviceValues: LocalDeviceSettings = $state(loadDeviceSettings())
  let devices: { inputs: MediaDeviceInfo[]; outputs: MediaDeviceInfo[] } =
    $state({ inputs: [], outputs: [] })
  let error = $state('')
  const quality = createMutation(() => ({
    mutationFn: (videoQuality: AccountSettings['video_quality']) =>
      api<AccountSettings>('/api/account/settings', {
        method: 'PUT',
        body: JSON.stringify({ video_quality: videoQuality }),
      }),
    onSuccess: onSettingsSaved,
  }))
  $effect(() => {
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', closeOnEscape)
    void listDevices().then((next) => {
      devices = next
    })
    return () => window.removeEventListener('keydown', closeOnEscape)
  })
  async function allowDevices() {
    error = ''
    try {
      devices = await requestAndListAudioDevices()
    } catch {
      error = 'Audio device access denied.'
    }
  }
  async function selectDevice(
    key: 'audioInputId' | 'audioOutputId',
    kind: 'audioinput' | 'audiooutput',
    value: string,
  ) {
    const next = { ...deviceValues, [key]: value }
    deviceValues = next
    saveDeviceSettings(next)
    if (!room) return
    error = ''
    try {
      await room.switchActiveDevice(kind, value || 'default', false)
    } catch {
      error =
        kind === 'audioinput'
          ? 'Could not switch microphone.'
          : 'This browser cannot switch output devices.'
    }
  }
  function setMicrophoneGain(value: number) {
    const next = { ...deviceValues, microphoneGain: value }
    deviceValues = next
    saveDeviceSettings(next)
    const track = room?.localParticipant.getTrackPublication(
      Track.Source.Microphone,
    )?.audioTrack
    if (!track) return
    void applyMicrophoneGain(track, value).catch(
      () =>
        (error = 'Could not change microphone volume in this browser.'),
    )
  }
  async function setNoiseSuppression(enabled: boolean) {
    const next = { ...deviceValues, noiseSuppression: enabled }
    deviceValues = next
    saveDeviceSettings(next)
    const track = room?.localParticipant.getTrackPublication(
      Track.Source.Microphone,
    )?.audioTrack
    if (!track) return
    error = ''
    try {
      await track.applyConstraints({
        noiseSuppression: enabled,
        voiceIsolation: enabled,
      })
    } catch {
      error =
        'This browser cannot change noise suppression during a call.'
    }
  }

  async function listDevices() {
    if (!navigator.mediaDevices?.enumerateDevices)
      return { inputs: [], outputs: [] }
    const devices = await navigator.mediaDevices.enumerateDevices()
    return {
      inputs: devices.filter((device) => device.kind === 'audioinput'),
      outputs: devices.filter((device) => device.kind === 'audiooutput'),
    }
  }
</script>

<div
  class="call-settings-backdrop"
  role="presentation"
  onmousedown={(event) => {
    if (event.target === event.currentTarget) onClose()
  }}
>
  <div
    class="call-settings-modal"
    role="dialog"
    aria-modal="true"
    aria-labelledby="call-settings-title"
  >
    <div class="section-heading">
      <div>
        <span class="section-kicker">During your call</span>
        <h2
          id="call-settings-title"
          class="font-display text-3xl font-semibold"
        >
          Call settings
        </h2>
      </div>
      <button class="mini-action" onclick={onClose} aria-label="Close"
        ><X size={18} /></button
      >
    </div>
    <button
      type="button"
      class="button-secondary compact mt-6"
      onclick={allowDevices}
      ><SlidersHorizontal size={17} /> Refresh devices</button
    >
    <div class="mt-5 space-y-4">
      <CallDeviceSelect
        label="Microphone"
        value={deviceValues.audioInputId}
        devices={devices.inputs}
        onChange={(value) => selectDevice('audioInputId', 'audioinput', value)}
      />
      <CallDeviceSelect
        label="Headphones or speakers"
        value={deviceValues.audioOutputId}
        devices={devices.outputs}
        onChange={(value) =>
          selectDevice('audioOutputId', 'audiooutput', value)}
        disabled={!('setSinkId' in HTMLMediaElement.prototype)}
      />
      <label class="range-setting">
        <span
          >Microphone volume <strong>{deviceValues.microphoneGain}%</strong
          ></span
        >
        <input
          type="range"
          min="0"
          max="200"
          step="5"
          value={deviceValues.microphoneGain}
          oninput={(event) =>
            setMicrophoneGain(Number(event.currentTarget.value))}
        />
      </label>
      <label class="toggle-setting">
        <span
          ><strong>Noise suppression</strong><small
            >Reduces steady background noise</small
          ></span
        >
        <input
          type="checkbox"
          checked={deviceValues.noiseSuppression}
          oninput={(event) => {
            void setNoiseSuppression(event.currentTarget.checked)
          }}
        />
      </label>
      <label class="field-label"
        >Screen sharing quality<select
          class="text-input"
          value={settings?.video_quality ?? 'high'}
          oninput={(event) =>
            quality.mutate(
              event.currentTarget.value as AccountSettings['video_quality'],
            )}
          disabled={quality.isPending}
          ><option value="low">720p · 60 fps</option><option value="high"
            >1080p · 60 fps</option
          ></select
        ></label
      >
      <p class="settings-hint">
        Audio settings apply immediately. Microphone levels
        above 100% can increase noise. Video quality applies
        the next time you share your screen.
      </p>
      {#if error || quality.error}
        <p class="error-note">
          {error || quality.error?.message}
        </p>
      {/if}
    </div>
  </div>
</div>
