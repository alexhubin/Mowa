<script lang="ts">
  import { onMount } from 'svelte'

  import { createQuery, useQueryClient } from '@tanstack/svelte-query'

  import Navigate from '../ui/Navigate.svelte'
  import { SlidersHorizontal } from '@lucide/svelte'
  import { api, currentUser, type AccountSettings } from '../api'
  import {
    loadDeviceSettings,
    requestAndListAudioDevices,
    saveDeviceSettings,
    type LocalDeviceSettings,
  } from '../deviceSettings'

  import { initials } from '../utils'
  import PasskeySettings from './PasskeySettings.svelte'
  import ProfileForm from './ProfileForm.svelte'
  import PasswordForm from './PasswordForm.svelte'
  import QualityForm from './QualityForm.svelte'
  import DeviceSelect from './DeviceSelect.svelte'
  const queryClient = useQueryClient()
  const userQuery = createQuery(() => ({
    queryKey: ['me'],
    queryFn: currentUser,
  }))
  const user = $derived(userQuery.data)
  const userLoading = $derived(userQuery.isLoading)
  const settings = createQuery(() => ({
    queryKey: ['account-settings'],
    queryFn: () => api<AccountSettings>('/api/account/settings'),
    enabled: Boolean(user && !user.must_change_password),
  }))
  let devices: Devices = $state({ inputs: [], outputs: [] })
  let deviceValues: LocalDeviceSettings = $state(loadDeviceSettings())
  let deviceError = $state('')
  onMount(() => {
    if (!navigator.mediaDevices?.enumerateDevices) return
    void navigator.mediaDevices.enumerateDevices().then(
      (items) =>
        (devices = {
          inputs: items.filter((item) => item.kind === 'audioinput'),
          outputs: items.filter((item) => item.kind === 'audiooutput'),
        }),
    )
  })
  async function allowDevices() {
    deviceError = ''
    try {
      devices = await requestAndListAudioDevices()
    } catch {
      deviceError =
        'Браузер не дал доступ к аудиоустройствам. Проверьте разрешение микрофона.'
    }
  }
  function updateDevice<K extends keyof LocalDeviceSettings>(
    key: K,
    value: LocalDeviceSettings[K],
  ) {
    const next = { ...deviceValues, [key]: value }
    deviceValues = next
    saveDeviceSettings(next)
  }

  type Devices = { inputs: MediaDeviceInfo[]; outputs: MediaDeviceInfo[] }
</script>

{#if userLoading}
  <main class="app-page">
    <div class="skeleton h-[32rem]"></div>
  </main>
{:else if !user}
  <Navigate to="/login" />
{:else if user.must_change_password}
  <Navigate to="/first-password" />
{:else}
  <main class="app-page settings-page">
    <h1 class="page-title">Настройки</h1>
    <div class="settings-stack">
      <section class="settings-card">
        <h2>Аккаунт</h2>
        <div class="account-avatar-row">
          <span class="account-avatar">{initials(user.display_name)}</span><span
            ><strong>{user.display_name}</strong><small>@{user.username}</small
            ></span
          >
        </div>
        <ProfileForm
          {user}
          onSaved={(next) => queryClient.setQueryData(['me'], next)}
        />
        <div class="settings-divider"></div>
        <PasswordForm />
      </section>

      <PasskeySettings />

      <section class="settings-card">
        <div class="settings-card-heading">
          <h2>Аудио</h2>
          <button
            type="button"
            class="button-secondary compact"
            onclick={allowDevices}
            ><SlidersHorizontal size={16} /> Обновить устройства</button
          >
        </div>
        <div class="settings-fields">
          <DeviceSelect
            label="Микрофон"
            value={deviceValues.audioInputId}
            devices={devices.inputs}
            onChange={(value) => updateDevice('audioInputId', value)}
          />
          <DeviceSelect
            label="Динамики"
            value={deviceValues.audioOutputId}
            devices={devices.outputs}
            onChange={(value) => updateDevice('audioOutputId', value)}
            disabled={!('setSinkId' in HTMLMediaElement.prototype)}
          />
          <label class="range-setting">
            <span
              >Громкость микрофона <strong
                >{deviceValues.microphoneGain}%</strong
              ></span
            >
            <input
              type="range"
              min="0"
              max="200"
              step="5"
              value={deviceValues.microphoneGain}
              oninput={(event) =>
                updateDevice(
                  'microphoneGain',
                  Number(event.currentTarget.value),
                )}
            />
          </label>
          <label class="toggle-setting">
            <span
              ><strong>Шумоподавление</strong><small
                >Убирает постоянный фоновый шум средствами браузера</small
              ></span
            >
            <input
              type="checkbox"
              checked={deviceValues.noiseSuppression}
              oninput={(event) =>
                updateDevice('noiseSuppression', event.currentTarget.checked)}
            />
          </label>
          <p class="settings-hint">
            Значения выше 100% усиливают голос перед отправкой, но могут также
            усилить шум.
          </p>
          {#if !('setSinkId' in HTMLMediaElement.prototype)}
            <p class="settings-hint">
              Этот браузер использует системное устройство вывода.
            </p>
          {/if}
          {#if deviceError}
            <p class="error-note">{deviceError}</p>
          {/if}
        </div>
      </section>

      <section class="settings-card">
        <h2>Демонстрация экрана</h2>
        {#if settings.data}
          <QualityForm
            value={settings.data.video_quality}
            onSaved={(next) =>
              queryClient.setQueryData(['account-settings'], next)}
          />
        {/if}
      </section>
    </div>
  </main>
{/if}
