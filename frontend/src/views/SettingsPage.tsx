import { useEffect, useState, type FormEvent } from 'react'
import { Navigate } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, KeyRound, Plus, SlidersHorizontal, Trash2 } from 'lucide-react'
import { api, currentUser, type AccountSettings, type User } from '../api'
import { loadDeviceSettings, requestAndListAudioDevices, saveDeviceSettings, type LocalDeviceSettings } from '../deviceSettings'
import { passkeysSupported, registerPasskey, type Passkey } from '../passkeys'
import { initials } from '../utils'

type Devices = { inputs: MediaDeviceInfo[]; outputs: MediaDeviceInfo[] }

export function SettingsPage() {
  const queryClient = useQueryClient()
  const { data: user, isLoading: userLoading } = useQuery({ queryKey: ['me'], queryFn: currentUser })
  const settings = useQuery({ queryKey: ['account-settings'], queryFn: () => api<AccountSettings>('/api/account/settings'), enabled: Boolean(user && !user.must_change_password) })
  const [devices, setDevices] = useState<Devices>({ inputs: [], outputs: [] })
  const [deviceValues, setDeviceValues] = useState<LocalDeviceSettings>(loadDeviceSettings)
  const [deviceError, setDeviceError] = useState('')

  useEffect(() => {
    if (!navigator.mediaDevices?.enumerateDevices) return
    void navigator.mediaDevices.enumerateDevices().then((items) => setDevices({ inputs: items.filter((item) => item.kind === 'audioinput'), outputs: items.filter((item) => item.kind === 'audiooutput') }))
  }, [])

  if (userLoading) return <main className="app-page"><div className="skeleton h-[32rem]" /></main>
  if (!user) return <Navigate to="/login" />
  if (user.must_change_password) return <Navigate to="/first-password" />

  async function allowDevices() {
    setDeviceError('')
    try { setDevices(await requestAndListAudioDevices()) }
    catch { setDeviceError('Браузер не дал доступ к аудиоустройствам. Проверьте разрешение микрофона.') }
  }

  function updateDevice<K extends keyof LocalDeviceSettings>(key: K, value: LocalDeviceSettings[K]) {
    const next = { ...deviceValues, [key]: value }
    setDeviceValues(next)
    saveDeviceSettings(next)
  }

  return (
    <main className="app-page settings-page">
      <h1 className="page-title">Настройки</h1>
      <div className="settings-stack">
        <section className="settings-card">
          <h2>Аккаунт</h2>
          <div className="account-avatar-row"><span className="account-avatar">{initials(user.display_name)}</span><span><strong>{user.display_name}</strong><small>@{user.username}</small></span></div>
          <ProfileForm user={user} onSaved={(next) => queryClient.setQueryData(['me'], next)} />
          <div className="settings-divider" />
          <PasswordForm />
        </section>

        <PasskeySettings />

        <section className="settings-card">
          <div className="settings-card-heading"><h2>Аудио</h2><button type="button" className="button-secondary compact" onClick={allowDevices}><SlidersHorizontal size={16} /> Обновить устройства</button></div>
          <div className="settings-fields">
            <DeviceSelect label="Микрофон" value={deviceValues.audioInputId} devices={devices.inputs} onChange={(value) => updateDevice('audioInputId', value)} />
            <DeviceSelect label="Динамики" value={deviceValues.audioOutputId} devices={devices.outputs} onChange={(value) => updateDevice('audioOutputId', value)} disabled={!('setSinkId' in HTMLMediaElement.prototype)} />
            <label className="range-setting">
              <span>Громкость микрофона <strong>{deviceValues.microphoneGain}%</strong></span>
              <input type="range" min="0" max="200" step="5" value={deviceValues.microphoneGain} onChange={(event) => updateDevice('microphoneGain', Number(event.target.value))} />
            </label>
            <label className="toggle-setting">
              <span><strong>Шумоподавление</strong><small>Убирает постоянный фоновый шум средствами браузера</small></span>
              <input type="checkbox" checked={deviceValues.noiseSuppression} onChange={(event) => updateDevice('noiseSuppression', event.target.checked)} />
            </label>
            <p className="settings-hint">Значения выше 100% усиливают голос перед отправкой, но могут также усилить шум.</p>
            {!('setSinkId' in HTMLMediaElement.prototype) && <p className="settings-hint">Этот браузер использует системное устройство вывода.</p>}
            {deviceError && <p className="error-note">{deviceError}</p>}
          </div>
        </section>

        <section className="settings-card">
          <h2>Демонстрация экрана</h2>
          {settings.data && <QualityForm value={settings.data.video_quality} onSaved={(next) => queryClient.setQueryData(['account-settings'], next)} />}
        </section>
      </div>
    </main>
  )
}

function PasskeySettings() {
  const queryClient = useQueryClient()
  const [name, setName] = useState('Мой passkey')
  const passkeys = useQuery({ queryKey: ['passkeys'], queryFn: () => api<Passkey[]>('/api/account/passkeys') })
  const create = useMutation({
    mutationFn: () => registerPasskey(name),
    onSuccess: (passkey) => {
      queryClient.setQueryData<Passkey[]>(['passkeys'], (current = []) => [passkey, ...current])
      setName('Мой passkey')
    },
  })
  const remove = useMutation({
    mutationFn: (id: string) => api<void>(`/api/account/passkeys/${id}`, { method: 'DELETE' }),
    onSuccess: (_, id) => queryClient.setQueryData<Passkey[]>(['passkeys'], (current = []) => current.filter((item) => item.id !== id)),
  })
  const supported = passkeysSupported()

  return <section className="settings-card">
    <h2>Вход по passkey</h2>
    <div className="passkey-intro"><KeyRound size={20} /><p>Входите без пароля через Touch ID, Face ID, Windows Hello или ключ безопасности.</p></div>
    {supported ? <>
      <form className="passkey-create" onSubmit={(event) => { event.preventDefault(); create.mutate() }}>
        <input className="text-input" value={name} onChange={(event) => setName(event.target.value)} maxLength={50} required aria-label="Название passkey" placeholder="Например, MacBook" />
        <button className="button-primary compact" disabled={create.isPending || !name.trim()}><Plus size={16} /> {create.isPending ? 'Подтвердите…' : 'Добавить'}</button>
      </form>
      {(passkeys.error || create.error || remove.error) && <p className="error-note">{passkeys.error?.message || create.error?.message || remove.error?.message}</p>}
      <div className="passkey-list">
        {passkeys.isLoading && <div className="skeleton h-16" />}
        {passkeys.data?.map((passkey) => <div className="passkey-row" key={passkey.id}>
          <span className="passkey-icon"><KeyRound size={17} /></span>
          <span><strong>{passkey.name}</strong><small>{passkey.last_used_at ? `Последний вход: ${formatPasskeyDate(passkey.last_used_at)}` : `Добавлен: ${formatPasskeyDate(passkey.created_at)}`}</small></span>
          <button type="button" className="mini-action" aria-label={`Удалить ${passkey.name}`} title="Удалить passkey" disabled={remove.isPending} onClick={() => { if (window.confirm(`Удалить passkey «${passkey.name}»?`)) remove.mutate(passkey.id) }}><Trash2 size={16} /></button>
        </div>)}
        {!passkeys.isLoading && passkeys.data?.length === 0 && <p className="settings-hint">Passkey пока не добавлены. Пароль останется доступен как резервный способ входа.</p>}
      </div>
    </> : <p className="settings-hint">Passkey недоступен: нужен современный браузер и защищённое HTTPS-соединение.</p>}
  </section>
}

function formatPasskeyDate(value: string) {
  return new Intl.DateTimeFormat('ru-RU', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}

function ProfileForm({ user, onSaved }: { user: User; onSaved: (user: User) => void }) {
  const [username, setUsername] = useState(user.username)
  const [displayName, setDisplayName] = useState(user.display_name)
  const mutation = useMutation({ mutationFn: () => api<User>('/api/account/profile', { method: 'PATCH', body: JSON.stringify({ username, display_name: displayName }) }), onSuccess: onSaved })
  return <form className="settings-fields" onSubmit={(event) => { event.preventDefault(); mutation.mutate() }}>
    <label className="field-label">Имя<input className="text-input" value={displayName} onChange={(event) => setDisplayName(event.target.value)} minLength={2} maxLength={40} required /></label>
    <label className="field-label">Ник<input className="text-input" value={username} onChange={(event) => setUsername(event.target.value.toLowerCase())} minLength={3} maxLength={32} pattern="[a-z0-9_]+" required /></label>
    <label className="field-label">Email<input className="text-input" value={user.email} readOnly /></label>
    {mutation.error && <p className="error-note">{mutation.error.message}</p>}
    <button className="button-primary compact settings-save" disabled={mutation.isPending}>{mutation.isSuccess ? <><Check size={16} /> Сохранено</> : 'Сохранить'}</button>
  </form>
}

function PasswordForm() {
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const mutation = useMutation({ mutationFn: () => api<void>('/api/account/password', { method: 'PUT', body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }) }), onSuccess: () => { setCurrentPassword(''); setNewPassword('') } })
  function submit(event: FormEvent) { event.preventDefault(); mutation.mutate() }
  return <form className="settings-fields" onSubmit={submit}>
    <h3>Изменить пароль</h3>
    <label className="field-label">Текущий пароль<input className="text-input" type="password" autoComplete="current-password" value={currentPassword} onChange={(event) => setCurrentPassword(event.target.value)} required /></label>
    <label className="field-label">Новый пароль<input className="text-input" type="password" autoComplete="new-password" minLength={8} maxLength={128} value={newPassword} onChange={(event) => setNewPassword(event.target.value)} required /></label>
    {mutation.error && <p className="error-note">{mutation.error.message}</p>}
    <button className="button-secondary compact settings-save" disabled={mutation.isPending}>{mutation.isSuccess ? <><Check size={16} /> Пароль изменён</> : 'Изменить пароль'}</button>
  </form>
}

function QualityForm({ value, onSaved }: { value: AccountSettings['video_quality']; onSaved: (settings: AccountSettings) => void }) {
  const [quality, setQuality] = useState(value)
  const mutation = useMutation({ mutationFn: () => api<AccountSettings>('/api/account/settings', { method: 'PUT', body: JSON.stringify({ video_quality: quality }) }), onSuccess: onSaved })
  return <form className="settings-fields" onSubmit={(event) => { event.preventDefault(); mutation.mutate() }}>
    <label className="field-label">Качество<select className="text-input" value={quality} onChange={(event) => setQuality(event.target.value as AccountSettings['video_quality'])}><option value="low">720p · 30 кадров/с</option><option value="high">1080p · 30 кадров/с</option></select></label>
    <button className="button-primary compact settings-save" disabled={mutation.isPending}>{mutation.isSuccess ? <><Check size={16} /> Сохранено</> : 'Сохранить'}</button>
  </form>
}

function DeviceSelect({ label, value, devices, onChange, disabled }: { label: string; value: string; devices: MediaDeviceInfo[]; onChange: (value: string) => void; disabled?: boolean }) {
  return <label className="field-label">{label}<select className="text-input" value={value} onChange={(event) => onChange(event.target.value)} disabled={disabled}><option value="">Системное устройство</option>{devices.map((device, index) => <option key={device.deviceId || index} value={device.deviceId}>{device.label || `${label} ${index + 1}`}</option>)}</select></label>
}
