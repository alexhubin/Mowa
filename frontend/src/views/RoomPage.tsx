import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, Navigate, useNavigate, useParams } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Check,
  Copy,
  Maximize2,
  Mic,
  MicOff,
  Minimize2,
  MonitorUp,
  PhoneOff,
  Radio,
  Settings,
  SlidersHorizontal,
  X,
} from 'lucide-react'
import {
  ConnectionState,
  Room,
  RoomEvent,
  ScreenSharePresets,
  Track,
  type Participant,
  type TrackPublication,
} from 'livekit-client'
import { api, currentUser, type AccountSettings, type DirectCall, type RoomInfo, type RoomToken } from '../api'
import { loadDeviceSettings, requestAndListAudioDevices, saveDeviceSettings, type LocalDeviceSettings } from '../deviceSettings'
import { applyMicrophoneGain } from '../microphoneProcessor'
import { initials, inviteURL } from '../utils'

export function RoomPage() {
  const { inviteCode } = useParams({ from: '/r/$inviteCode' })
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const audioHost = useRef<HTMLDivElement>(null)
  const stageRef = useRef<HTMLElement>(null)
  const activeCall = useRef<Room | null>(null)
  const directCallID = useRef<string | null>(null)
  const [call, setCall] = useState<Room | null>(null)
  const [, render] = useState(0)
  const [copied, setCopied] = useState(false)
  const [controlError, setControlError] = useState('')
  const [controlBusy, setControlBusy] = useState(false)
  const [callSettingsOpen, setCallSettingsOpen] = useState(false)
  const [fullscreen, setFullscreen] = useState(false)

  const { data: user, isLoading: userLoading } = useQuery({ queryKey: ['me'], queryFn: currentUser })
  const roomQuery = useQuery({
    queryKey: ['room', inviteCode],
    queryFn: () => api<RoomInfo>(`/api/rooms/${inviteCode}`),
    enabled: Boolean(user && !user.must_change_password),
  })
  const settingsQuery = useQuery({
    queryKey: ['account-settings'],
    queryFn: () => api<AccountSettings>('/api/account/settings'),
    enabled: Boolean(user && !user.must_change_password),
  })
  const callsQuery = useQuery({
    queryKey: ['calls'],
    queryFn: () => api<DirectCall[]>('/api/calls'),
    enabled: Boolean(user && !user.must_change_password),
    refetchInterval: 2_000,
  })

  useEffect(() => {
    const openSettings = () => setCallSettingsOpen(true)
    window.addEventListener('mova:open-call-settings', openSettings)
    return () => window.removeEventListener('mova:open-call-settings', openSettings)
  }, [])

  useEffect(() => {
    const syncFullscreen = () => setFullscreen(document.fullscreenElement === stageRef.current)
    document.addEventListener('fullscreenchange', syncFullscreen)
    return () => document.removeEventListener('fullscreenchange', syncFullscreen)
  }, [])

  useEffect(() => {
    if (roomQuery.data?.kind !== 'direct') {
      directCallID.current = null
      return
    }
    const matching = callsQuery.data?.find((item) => item.invite_code === inviteCode)
    if (matching) directCallID.current = matching.id
  }, [callsQuery.data, inviteCode, roomQuery.data?.kind])

  useEffect(() => {
    const closeActiveCall = () => {
      const room = activeCall.current
      activeCall.current = null
      if (room) {
        stopLocalMedia(room)
        void room.disconnect()
      }
      endDirectCall(directCallID)
    }

    window.addEventListener('pagehide', closeActiveCall)
    window.addEventListener('beforeunload', closeActiveCall)
    window.addEventListener('freeze', closeActiveCall)

    return () => {
      window.removeEventListener('pagehide', closeActiveCall)
      window.removeEventListener('beforeunload', closeActiveCall)
      window.removeEventListener('freeze', closeActiveCall)
      closeActiveCall()
    }
  }, [])

  const join = useMutation({
    mutationFn: async () => {
      const credentials = await api<RoomToken>(`/api/rooms/${inviteCode}/token`, { method: 'POST' })
      const nextCall = new Room({ adaptiveStream: true, dynacast: true, disconnectOnPageLeave: true })
      activeCall.current = nextCall

      const refresh = () => render((value) => value + 1)
      const events = [
        RoomEvent.ParticipantConnected,
        RoomEvent.ParticipantDisconnected,
        RoomEvent.TrackPublished,
        RoomEvent.TrackUnpublished,
        RoomEvent.TrackMuted,
        RoomEvent.TrackUnmuted,
        RoomEvent.ActiveSpeakersChanged,
        RoomEvent.ConnectionStateChanged,
        RoomEvent.LocalTrackPublished,
        RoomEvent.LocalTrackUnpublished,
      ] as const
      events.forEach((event) => nextCall.on(event, refresh))

      nextCall.on(RoomEvent.TrackSubscribed, (track) => {
        refresh()
        if (track.kind === Track.Kind.Audio && audioHost.current) {
          const element = track.attach()
          element.dataset.movaAudio = track.sid ?? ''
          audioHost.current.appendChild(element)
        }
      })
      nextCall.on(RoomEvent.TrackUnsubscribed, (track) => {
        track.detach().forEach((element) => element.remove())
        refresh()
      })

      try {
        await nextCall.connect(credentials.server_url, credentials.token)
      } catch (error) {
        if (activeCall.current === nextCall) activeCall.current = null
        stopLocalMedia(nextCall)
        void nextCall.disconnect()
        throw error
      }

      if (activeCall.current !== nextCall) return nextCall
      setCall(nextCall)
      await nextCall.startAudio()
      if (activeCall.current !== nextCall) return nextCall

      const devices = loadDeviceSettings()
      if (devices.audioOutputId) {
        await nextCall.switchActiveDevice('audiooutput', devices.audioOutputId, false).catch(() => undefined)
      }
      try {
        const publication = await nextCall.localParticipant.setMicrophoneEnabled(true, microphoneCaptureOptions(devices))
        if (devices.microphoneGain !== 100) {
          await applyMicrophoneGain(publication?.audioTrack, devices.microphoneGain)
        }
      } catch {
        setControlError('Микрофон не включён. Разрешите доступ в настройках браузера и попробуйте ещё раз.')
      }
      return nextCall
    },
    onError: (error) => setControlError(error instanceof Error ? error.message : 'Не удалось подключиться'),
  })

  const participants = useMemo(() => {
    if (!call) return []
    return [call.localParticipant, ...Array.from(call.remoteParticipants.values())]
  }, [call, call?.remoteParticipants.size, call?.state]) // eslint-disable-line react-hooks/exhaustive-deps

  const activeScreenShare = participants
    .map((participant) => ({
      participant,
      publication: participant.getTrackPublication(Track.Source.ScreenShare),
    }))
    .find(({ publication }) => publication !== undefined)
  const screenPublication = activeScreenShare?.publication
  const remoteScreenShareActive = Boolean(
    call && activeScreenShare && activeScreenShare.participant.identity !== call.localParticipant.identity,
  )

  useEffect(() => {
    if (!screenPublication && document.fullscreenElement === stageRef.current) {
      void document.exitFullscreen()
    }
  }, [screenPublication])

  async function toggleMic() {
    if (!call) return
    setControlBusy(true)
    setControlError('')
    try {
      const enable = !call.localParticipant.isMicrophoneEnabled
      const devices = loadDeviceSettings()
      const publication = await call.localParticipant.setMicrophoneEnabled(
        enable,
        enable ? microphoneCaptureOptions(devices) : undefined,
      )
      if (enable && devices.microphoneGain !== 100) {
        await applyMicrophoneGain(publication?.audioTrack, devices.microphoneGain)
      }
      render((value) => value + 1)
    } catch (error) {
      setControlError(error instanceof Error ? error.message : 'Нет доступа к микрофону')
    } finally {
      setControlBusy(false)
    }
  }

  async function toggleScreen() {
    if (!call) return
    setControlBusy(true)
    setControlError('')
    try {
      const enable = !call.localParticipant.isScreenShareEnabled
      const remoteSharer = enable
        ? Array.from(call.remoteParticipants.values()).find((participant) =>
            participant.getTrackPublication(Track.Source.ScreenShare),
          )
        : undefined
      if (remoteSharer) {
        setControlError(`${remoteSharer.name || 'Другой участник'} уже демонстрирует экран`)
        return
      }
      const quality = settingsQuery.data?.video_quality ?? 'high'
      const preset = quality === 'low'
        ? ScreenSharePresets.h720fps30
        : ScreenSharePresets.h1080fps30
      await call.localParticipant.setScreenShareEnabled(
        enable,
        enable ? { resolution: preset.resolution, contentHint: 'detail' } : undefined,
        enable
          ? {
              screenShareEncoding: preset.encoding,
              simulcast: true,
              videoCodec: 'vp9',
              backupCodec: true,
              scalabilityMode: 'L3T3_KEY',
              degradationPreference: 'maintain-resolution',
            }
          : undefined,
      )
      render((value) => value + 1)
    } catch (error) {
      setControlError(error instanceof Error ? error.message : 'Демонстрация экрана недоступна в этом браузере')
    } finally {
      setControlBusy(false)
    }
  }

  async function copyInvite() {
    try {
      await navigator.clipboard.writeText(inviteURL(inviteCode))
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1800)
    } catch {
      setControlError('Не удалось скопировать ссылку')
    }
  }

  async function toggleFullscreen() {
    setControlError('')
    try {
      if (document.fullscreenElement) await document.exitFullscreen()
      else await stageRef.current?.requestFullscreen()
    } catch {
      setControlError('Полноэкранный режим недоступен в этом браузере')
    }
  }

  async function leave() {
    const room = activeCall.current
    activeCall.current = null
    if (room) {
      stopLocalMedia(room)
      await room.disconnect()
    }
    const callID = directCallID.current
    directCallID.current = null
    if (callID) {
      await api<void>(`/api/calls/${callID}/end`, { method: 'POST' }).catch(() => undefined)
    }
    setCall(null)
    await navigate({ to: '/' })
  }

  if (userLoading) return <main className="page-shell py-20"><div className="skeleton h-[65dvh]" /></main>
  if (!user) {
    const next = encodeURIComponent(`/r/${inviteCode}`)
    return (
      <main className="page-shell grid min-h-[72dvh] place-items-center text-center">
        <section className="max-w-xl">
          <div className="eyebrow">Вас пригласили</div>
          <h1 className="font-display mt-5 text-5xl font-semibold tracking-[-0.05em]">Сначала представьтесь</h1>
          <p className="mt-4 text-lg text-ink-muted">Участники комнаты должны видеть, кто присоединился к разговору.</p>
          <div className="mt-8 flex justify-center"><a href={`/login?next=${next}`} className="button-primary">Войти</a></div>
        </section>
      </main>
    )
  }
  if (user.must_change_password) return <Navigate to="/first-password" />
  if (roomQuery.isLoading) return <main className="page-shell py-20"><div className="skeleton h-[65dvh]" /></main>
  if (roomQuery.error || !roomQuery.data) {
    return (
      <main className="page-shell grid min-h-[72dvh] place-items-center text-center">
        <div><div className="font-mono text-sm text-accent">ROOM NOT FOUND</div><h1 className="font-display mt-4 text-5xl font-semibold">Комната не отвечает</h1><p className="mt-3 text-ink-muted">Проверьте ссылку или попросите новое приглашение.</p><Link to="/" className="button-primary mt-7">На главную</Link></div>
      </main>
    )
  }

  const connected = call?.state === ConnectionState.Connected
  const micEnabled = call?.localParticipant.isMicrophoneEnabled ?? false
  const screenEnabled = call?.localParticipant.isScreenShareEnabled ?? false

  return (
    <main className="room-page">
      <div ref={audioHost} className="hidden" aria-hidden="true" />
      <div className="room-topbar">
        <Link to="/" className="brand room-brand"><span className="brand-dot" /><span>mowa</span></Link>
        <h1>{roomQuery.data.name}</h1>
        <code>{inviteCode}</code>
        <button className="button-secondary compact" onClick={copyInvite}>{copied ? <Check size={15} /> : <Copy size={15} />} {copied ? 'Скопировано' : 'Копировать ссылку'}</button>
        <span className="room-topbar-spacer" />
        <span className={connected ? 'broadcast-status online' : 'broadcast-status'}><i />{connected ? 'в эфире' : 'комната готова'}</span>
      </div>

      {!connected ? (
        <section className="join-stage">
          <div className="join-visual" aria-hidden="true">
            <div className="avatar-preview">{initials(user.display_name)}</div>
            <div className="ring ring-one" /><div className="ring ring-two" />
          </div>
          <div className="max-w-lg text-center">
            <h2 className="font-display text-4xl font-semibold tracking-[-0.045em] sm:text-5xl">Готовы подключиться?</h2>
            <p className="mt-4 leading-relaxed text-ink-muted">Браузер попросит доступ к микрофону. Камера не включается.</p>
            {controlError && <p className="error-note mt-5" role="alert">{controlError}</p>}
            <button className="button-primary mt-7" onClick={() => join.mutate()} disabled={join.isPending}>
              <Radio size={19} /> {join.isPending ? 'Подключаем…' : 'Войти в разговор'}
            </button>
          </div>
        </section>
      ) : (
        <div className="call-grid">
          <section ref={stageRef} className="stage-panel">
            {screenPublication ? (
              <>
                <ScreenTrack publication={screenPublication} />
                <button className="fullscreen-button" onClick={toggleFullscreen} aria-label={fullscreen ? 'Выйти из полноэкранного режима' : 'Развернуть трансляцию на весь экран'} title={fullscreen ? 'Выйти из полноэкранного режима' : 'На весь экран'}>
                  {fullscreen ? <Minimize2 size={18} /> : <Maximize2 size={18} />}<span>{fullscreen ? 'Свернуть' : 'На весь экран'}</span>
                </button>
              </>
            ) : (
              <div className="empty-stage">
                <h2>Никто не демонстрирует экран</h2>
                <p>Нажмите кнопку с монитором внизу, чтобы показать свой</p>
              </div>
            )}
          </section>

          <aside className="participants-panel">
            <div className="participants-list">
              {participants.map((participant) => <ParticipantRow key={participant.identity} participant={participant} local={participant.identity === call.localParticipant.identity} />)}
            </div>
          </aside>
        </div>
      )}

      {connected && (
        <div className="control-dock" aria-label="Управление звонком">
          <button className={`call-control ${!micEnabled ? 'danger' : ''}`} onClick={toggleMic} disabled={controlBusy} aria-label={micEnabled ? 'Выключить микрофон' : 'Включить микрофон'} title={micEnabled ? 'Выключить микрофон' : 'Включить микрофон'}>
            {micEnabled ? <Mic size={21} /> : <MicOff size={21} />}
          </button>
          <button className={`call-control wide ${screenEnabled ? 'active' : ''}`} onClick={toggleScreen} disabled={controlBusy || remoteScreenShareActive} aria-label={screenEnabled ? 'Остановить показ экрана' : remoteScreenShareActive ? 'Другой участник уже демонстрирует экран' : 'Показать экран'} title={remoteScreenShareActive ? `${activeScreenShare?.participant.name || 'Другой участник'} уже демонстрирует экран` : undefined}>
            <MonitorUp size={21} /><span>{screenEnabled ? 'Остановить' : remoteScreenShareActive ? 'Занято' : 'Экран'}</span>
          </button>
          <button className="call-control" onClick={() => setCallSettingsOpen(true)} aria-label="Настройки звонка" title="Настройки звонка">
            <Settings size={21} />
          </button>
          <button className="call-control danger" onClick={leave} aria-label="Выйти из комнаты" title="Выйти">
            <PhoneOff size={21} />
          </button>
        </div>
      )}
      {connected && controlError && <div className="room-error" role="alert">{controlError}</div>}
      {callSettingsOpen && <CallSettingsModal room={call} settings={settingsQuery.data} onClose={() => setCallSettingsOpen(false)} onSettingsSaved={(next) => queryClient.setQueryData(['account-settings'], next)} />}
    </main>
  )
}

function CallSettingsModal({ room, settings, onClose, onSettingsSaved }: { room: Room | null; settings?: AccountSettings; onClose: () => void; onSettingsSaved: (settings: AccountSettings) => void }) {
  const [deviceValues, setDeviceValues] = useState<LocalDeviceSettings>(loadDeviceSettings)
  const [devices, setDevices] = useState<{ inputs: MediaDeviceInfo[]; outputs: MediaDeviceInfo[] }>({ inputs: [], outputs: [] })
  const [error, setError] = useState('')
  const quality = useMutation({
    mutationFn: (videoQuality: AccountSettings['video_quality']) => api<AccountSettings>('/api/account/settings', { method: 'PUT', body: JSON.stringify({ video_quality: videoQuality }) }),
    onSuccess: onSettingsSaved,
  })

  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => { if (event.key === 'Escape') onClose() }
    window.addEventListener('keydown', closeOnEscape)
    void listDevices().then(setDevices)
    return () => window.removeEventListener('keydown', closeOnEscape)
  }, [onClose])

  async function allowDevices() {
    setError('')
    try {
      setDevices(await requestAndListAudioDevices())
    } catch {
      setError('Браузер не дал доступ к аудиоустройствам.')
    }
  }

  async function selectDevice(key: 'audioInputId' | 'audioOutputId', kind: 'audioinput' | 'audiooutput', value: string) {
    const next = { ...deviceValues, [key]: value }
    setDeviceValues(next)
    saveDeviceSettings(next)
    if (!room) return
    setError('')
    try {
      await room.switchActiveDevice(kind, value || 'default', false)
    } catch {
      setError(kind === 'audioinput' ? 'Не удалось переключить микрофон.' : 'Этот браузер не поддерживает переключение устройства вывода.')
    }
  }

  function setMicrophoneGain(value: number) {
    const next = { ...deviceValues, microphoneGain: value }
    setDeviceValues(next)
    saveDeviceSettings(next)
    const track = room?.localParticipant.getTrackPublication(Track.Source.Microphone)?.audioTrack
    if (!track) return
    void applyMicrophoneGain(track, value).catch(() => setError('Не удалось изменить громкость микрофона в этом браузере.'))
  }

  async function setNoiseSuppression(enabled: boolean) {
    const next = { ...deviceValues, noiseSuppression: enabled }
    setDeviceValues(next)
    saveDeviceSettings(next)
    const track = room?.localParticipant.getTrackPublication(Track.Source.Microphone)?.audioTrack
    if (!track) return
    setError('')
    try {
      await track.applyConstraints({ noiseSuppression: enabled, voiceIsolation: enabled })
    } catch {
      setError('Браузер не поддерживает изменение шумоподавления во время звонка.')
    }
  }

  return (
    <div className="call-settings-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
      <section className="call-settings-modal" role="dialog" aria-modal="true" aria-labelledby="call-settings-title">
        <div className="section-heading"><div><span className="section-kicker">Без выхода из комнаты</span><h2 id="call-settings-title" className="font-display text-3xl font-semibold">Настройки звонка</h2></div><button className="mini-action" onClick={onClose} aria-label="Закрыть"><X size={18} /></button></div>
        <button type="button" className="button-secondary compact mt-6" onClick={allowDevices}><SlidersHorizontal size={17} /> Обновить устройства</button>
        <div className="mt-5 space-y-4">
          <CallDeviceSelect label="Микрофон" value={deviceValues.audioInputId} devices={devices.inputs} onChange={(value) => selectDevice('audioInputId', 'audioinput', value)} />
          <CallDeviceSelect label="Наушники или динамики" value={deviceValues.audioOutputId} devices={devices.outputs} onChange={(value) => selectDevice('audioOutputId', 'audiooutput', value)} disabled={!('setSinkId' in HTMLMediaElement.prototype)} />
          <label className="range-setting">
            <span>Громкость микрофона <strong>{deviceValues.microphoneGain}%</strong></span>
            <input type="range" min="0" max="200" step="5" value={deviceValues.microphoneGain} onChange={(event) => setMicrophoneGain(Number(event.target.value))} />
          </label>
          <label className="toggle-setting">
            <span><strong>Шумоподавление</strong><small>Убирает постоянный фоновый шум средствами браузера</small></span>
            <input type="checkbox" checked={deviceValues.noiseSuppression} onChange={(event) => { void setNoiseSuppression(event.target.checked) }} />
          </label>
          <label className="field-label">Качество демонстрации экрана<select className="text-input" value={settings?.video_quality ?? 'high'} onChange={(event) => quality.mutate(event.target.value as AccountSettings['video_quality'])} disabled={quality.isPending}><option value="low">720p · 30 кадров/с</option><option value="high">1080p · 30 кадров/с</option></select></label>
          <p className="settings-hint">Аудионастройки применяются сразу, без выхода из разговора. Значения громкости выше 100% могут также усилить шум. Качество применяется при следующем запуске демонстрации экрана.</p>
          {(error || quality.error) && <p className="error-note">{error || quality.error?.message}</p>}
        </div>
      </section>
    </div>
  )
}

function CallDeviceSelect({ label, value, devices, onChange, disabled }: { label: string; value: string; devices: MediaDeviceInfo[]; onChange: (value: string) => void; disabled?: boolean }) {
  return <label className="field-label">{label}<select className="text-input" value={value} onChange={(event) => onChange(event.target.value)} disabled={disabled}><option value="">Системное устройство</option>{devices.map((device, index) => <option key={device.deviceId || index} value={device.deviceId}>{device.label || `${label} ${index + 1}`}</option>)}</select></label>
}

async function listDevices() {
  if (!navigator.mediaDevices?.enumerateDevices) return { inputs: [], outputs: [] }
  const devices = await navigator.mediaDevices.enumerateDevices()
  return { inputs: devices.filter((device) => device.kind === 'audioinput'), outputs: devices.filter((device) => device.kind === 'audiooutput') }
}

function microphoneCaptureOptions(settings: LocalDeviceSettings) {
  return {
    deviceId: settings.audioInputId ? { exact: settings.audioInputId } : undefined,
    echoCancellation: true,
    autoGainControl: true,
    noiseSuppression: settings.noiseSuppression,
    voiceIsolation: settings.noiseSuppression,
  }
}

function stopLocalMedia(room: Room) {
  room.localParticipant.getTrackPublications().forEach((publication) => publication.track?.stop())
}

function endDirectCall(callID: { current: string | null }) {
  const id = callID.current
  callID.current = null
  if (!id) return
  void fetch(`/api/calls/${id}/end`, { method: 'POST', credentials: 'same-origin', keepalive: true })
}

function ParticipantRow({ participant, local }: { participant: Participant; local: boolean }) {
  const mic = participant.getTrackPublication(Track.Source.Microphone)
  const muted = !mic || mic.isMuted
  return (
    <div className={`participant-row ${participant.isSpeaking ? 'speaking' : ''}`}>
      <div className="participant-avatar">{initials(participant.name || participant.identity)}</div>
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-semibold">{participant.name || 'Участник'} {local && <span className="font-normal text-ink-muted">(вы)</span>}</div>
        <div className="mt-0.5 text-xs text-ink-muted">{participant.isSpeaking ? 'говорит' : muted ? 'микрофон выключен' : 'слушает'}</div>
      </div>
      <span className={muted ? 'mic-state muted' : 'mic-state'}>{muted ? <MicOff size={14} /> : <Mic size={14} />}</span>
    </div>
  )
}

function ScreenTrack({ publication }: { publication: TrackPublication }) {
  const video = useRef<HTMLVideoElement>(null)

  useEffect(() => {
    const element = video.current
    const track = publication.track
    if (!element || !track) return
    track.attach(element)
    return () => { track.detach(element) }
  }, [publication, publication.track])

  return <video ref={video} className="screen-video" autoPlay playsInline />
}
