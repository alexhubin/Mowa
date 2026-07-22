import { useEffect, useState, type FormEvent } from 'react'
import { Navigate } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, SlidersHorizontal } from 'lucide-react'
import { api, currentUser, type AccountSettings, type User } from '../api'
import { loadDeviceSettings, requestAndListAudioDevices, saveDeviceSettings, type LocalDeviceSettings } from '../deviceSettings'
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

  function updateDevice(key: keyof LocalDeviceSettings, value: string) {
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

        <section className="settings-card">
          <div className="settings-card-heading"><h2>Аудио</h2><button type="button" className="button-secondary compact" onClick={allowDevices}><SlidersHorizontal size={16} /> Обновить устройства</button></div>
          <div className="settings-fields">
            <DeviceSelect label="Микрофон" value={deviceValues.audioInputId} devices={devices.inputs} onChange={(value) => updateDevice('audioInputId', value)} />
            <DeviceSelect label="Динамики" value={deviceValues.audioOutputId} devices={devices.outputs} onChange={(value) => updateDevice('audioOutputId', value)} disabled={!('setSinkId' in HTMLMediaElement.prototype)} />
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
