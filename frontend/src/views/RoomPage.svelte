<script lang="ts">
  import { onMount } from 'svelte'
  import { applyRoomMix, applyTrackVolume, mixEvent } from '../playbackMix'
  import type { RemoteAudioTrack } from 'livekit-client'

  import {
    createQuery,
    createMutation,
    useQueryClient,
  } from '@tanstack/svelte-query'
  import { navigate } from '../navigation.svelte'
  import Navigate from '../ui/Navigate.svelte'
  import {
    Check,
    Copy,
    Maximize2,
    MessageSquare,
    Mic,
    MicOff,
    Minimize2,
    MonitorUp,
    PhoneOff,
    Radio,
    Settings,
    Send,
    Users,
  } from '@lucide/svelte'
  import {
    ConnectionState,
    Room,
    RoomEvent,
    Track,
  } from 'livekit-client'
  import {
    api,
    currentUser,
    type AccountSettings,
    type DirectCall,
    type RoomInfo,
    type RoomMessage,
    type RoomToken,
  } from '../api'
  import {
    loadDeviceSettings,
    type LocalDeviceSettings,
  } from '../deviceSettings'
  import { applyMicrophoneGain } from '../microphoneProcessor'
  import { initials, inviteURL } from '../utils'
  import CallSettingsModal from './CallSettingsModal.svelte'

  import ParticipantRow from './ParticipantRow.svelte'
  import ChatMessage from './ChatMessage.svelte'
  import ScreenTrack from './ScreenTrack.svelte'
  import { ScreenPublisher } from '../screenPublisher'
  let screenPublisher: ScreenPublisher | null = null
  let { inviteCode }: { inviteCode: string } = $props()
  const queryClient = useQueryClient()
  let audioHost: HTMLDivElement | null = $state.raw(null)
  let stageRef: HTMLElement | null = $state.raw(null)
  let sidePanelRef: HTMLElement | null = $state.raw(null)
  let messagesEndRef: HTMLDivElement | null = $state.raw(null)
  let activeCall: Room | null = $state.raw(null)
  let directCallID: string | null = $state.raw(null)
  let call: Room | null = $state.raw(null)
  // LiveKit mutates class instances outside Svelte; its events invalidate these snapshots.
  let revision = $state(0)
  let disposed = false
  let copied = $state(false)
  let controlError = $state('')
  let controlBusy = $state(false)
  let callSettingsOpen = $state(false)
  let fullscreen = $state(false)
  let sidePanel: 'participants' | 'chat' = $state('participants')
  let messageBody = $state('')
  const userQuery = createQuery(() => ({
    queryKey: ['me'],
    queryFn: currentUser,
  }))
  const account = $derived(userQuery.data)
  const user = $derived(account)
  const userLoading = $derived(userQuery.isLoading)
  const roomQuery = createQuery(() => ({
    queryKey: ['room', inviteCode],
    queryFn: () => api<RoomInfo>(`/api/rooms/${inviteCode}`),
    enabled: Boolean(user && !user.must_change_password),
  }))
  const settingsQuery = createQuery(() => ({
    queryKey: ['account-settings'],
    queryFn: () => api<AccountSettings>('/api/account/settings'),
    enabled: Boolean(account && !account.must_change_password),
  }))
  const callsQuery = createQuery(() => ({
    queryKey: ['calls'],
    queryFn: () => api<DirectCall[]>('/api/calls'),
    enabled: Boolean(account && !account.must_change_password),
  }))
  const connected = $derived.by(() => {
    void revision
    return call?.state === ConnectionState.Connected
  })
  const micEnabled = $derived.by(() => {
    void revision
    return call?.localParticipant.isMicrophoneEnabled ?? false
  })
  const screenEnabled = $derived.by(() => {
    void revision
    return call?.localParticipant.isScreenShareEnabled ?? false
  })
  const messagesQuery = createQuery(() => ({
    queryKey: ['room-messages', inviteCode],
    queryFn: () => api<RoomMessage[]>(`/api/rooms/${inviteCode}/messages`),
    enabled: connected,
  }))
  const sendMessage = createMutation(() => ({
    mutationFn: (body: string) =>
      api<RoomMessage>(`/api/rooms/${inviteCode}/messages`, {
        method: 'POST',
        body: JSON.stringify({ body }),
      }),
    onSuccess: (message) => {
      queryClient.setQueryData<RoomMessage[]>(
        ['room-messages', inviteCode],
        (current = []) =>
          current.some((item) => item.id === message.id)
            ? current
            : [...current, message],
      )
      messageBody = ''
    },
  }))
  onMount(() => {
    const openSettings = () => (callSettingsOpen = true)
    window.addEventListener('mova:open-call-settings', openSettings)
    return () =>
      window.removeEventListener('mova:open-call-settings', openSettings)
  })
  onMount(() => {
    const syncFullscreen = () =>
      (fullscreen = document.fullscreenElement === stageRef)
    document.addEventListener('fullscreenchange', syncFullscreen)
    return () =>
      document.removeEventListener('fullscreenchange', syncFullscreen)
  })
  $effect(() => {
    if (roomQuery.data?.kind !== 'direct') {
      directCallID = null
      return
    }
    const matching = callsQuery.data?.find(
      (item) => item.invite_code === inviteCode,
    )
    if (matching) directCallID = matching.id
  })
  onMount(() => {
    const closeActiveCall = () => {
      disposed = true
      const room = activeCall
      activeCall = null
      if (room) {
        stopLocalMedia(room)
        void room.disconnect()
      }
      endDirectCall(directCallID)
      directCallID = null
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
  })
  onMount(() => {
    const update = () => { if (activeCall) applyRoomMix(activeCall) }
    window.addEventListener(mixEvent, update)
    window.addEventListener('storage', update)
    return () => { window.removeEventListener(mixEvent, update); window.removeEventListener('storage', update) }
  })
  const join = createMutation(() => ({
    mutationFn: async () => {
      disposed = false
      const credentials = await api<RoomToken>(
        `/api/rooms/${inviteCode}/token`,
        { method: 'POST' },
      )
      if (disposed) return
      const nextCall = new Room({
        adaptiveStream: true,
        webAudioMix: true,
        dynacast: true,
        disconnectOnPageLeave: true,
      })
      activeCall = nextCall

      const refresh = () => { revision += 1; applyRoomMix(nextCall) }
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

      nextCall.on(RoomEvent.TrackSubscribed, (track, publication, participant) => {
        refresh()
        if (track.kind === Track.Kind.Audio && audioHost) {
          applyTrackVolume(track as RemoteAudioTrack, publication.source, undefined, participant.isSpeaking)
          const element = track.attach()
          applyTrackVolume(track as RemoteAudioTrack, publication.source, undefined, participant.isSpeaking)
          element.dataset.movaAudio = track.sid ?? ''
          // LiveKit owns the attached audio elements in this dedicated host.
          // eslint-disable-next-line svelte/no-dom-manipulating
          audioHost.appendChild(element)
        }
      })
      nextCall.on(RoomEvent.TrackUnsubscribed, (track) => {
        // Svelte owns video elements; only remove the audio elements we created.
        if (track.kind === Track.Kind.Audio) track.detach().forEach((element) => element.remove())
        refresh()
      })

      try {
        await nextCall.connect(credentials.server_url, credentials.token)
        if (activeCall === nextCall) {
          screenPublisher = new ScreenPublisher(nextCall, message => { controlError = message })
          await screenPublisher.announce()
        }
      } catch (error) {
        if (activeCall === nextCall) activeCall = null
        stopLocalMedia(nextCall)
        void nextCall.disconnect()
        throw error
      }

      if (activeCall !== nextCall) {
        stopLocalMedia(nextCall)
        await nextCall.disconnect()
        return nextCall
      }
      call = nextCall
      await nextCall.startAudio()
      if (activeCall !== nextCall) {
        stopLocalMedia(nextCall)
        await nextCall.disconnect()
        return nextCall
      }

      const devices = loadDeviceSettings()
      if (devices.audioOutputId) {
        await nextCall
          .switchActiveDevice('audiooutput', devices.audioOutputId, false)
          .catch(() => undefined)
      }
      if (activeCall !== nextCall) return nextCall
      try {
        const publication =
          await nextCall.localParticipant.setMicrophoneEnabled(
            true,
            microphoneCaptureOptions(devices),
          )
        if (activeCall !== nextCall) {
          stopLocalMedia(nextCall)
          await nextCall.disconnect()
          return nextCall
        }
        if (devices.microphoneGain !== 100) {
          await applyMicrophoneGain(
            publication?.audioTrack,
            devices.microphoneGain,
          )
        }
      } catch {
        controlError =
          'Microphone is off. Allow access in browser settings and try again.'
      }
      return nextCall
    },
    onError: (error) =>
      (controlError =
        error instanceof Error ? error.message : 'Could not connect'),
  }))
  const participants = $derived.by(() => {
    void revision
    if (!call) return []
    return [
      call.localParticipant,
      ...Array.from(call.remoteParticipants.values()),
    ]
  })
  const activeScreenShare = $derived.by(() => {
    void revision
    return participants
      .map((participant) => ({
        participant,
        publication: participant.getTrackPublication(Track.Source.ScreenShare),
      }))
      .find(({ publication }) => publication !== undefined)
  })
  const screenPublication = $derived(activeScreenShare?.publication)
  const remoteScreenShareActive = $derived.by(() =>
    Boolean(
      call &&
      activeScreenShare &&
      activeScreenShare.participant.identity !== call.localParticipant.identity,
    ),
  )
  $effect(() => {
    if (stageRef && !screenPublication && document.fullscreenElement === stageRef) {
      void document.exitFullscreen().catch(() => undefined)
    }
  })
  $effect(() => {
    if (!connected) return
    const events = new EventSource(`/api/rooms/${inviteCode}/messages/events`)
    const refreshMessages = () =>
      void queryClient.invalidateQueries({
        queryKey: ['room-messages', inviteCode],
      })
    events.addEventListener('messages', refreshMessages)
    return () => {
      events.removeEventListener('messages', refreshMessages)
      events.close()
    }
  })
  $effect(() => {
    void messagesQuery.data
    if (sidePanel !== 'chat') return
    messagesEndRef?.scrollIntoView({ block: 'end' })
  })
  async function toggleMic() {
    if (!call) return
    controlBusy = true
    controlError = ''
    try {
      const enable = !call.localParticipant.isMicrophoneEnabled
      const devices = loadDeviceSettings()
      const publication = await call.localParticipant.setMicrophoneEnabled(
        enable,
        enable ? microphoneCaptureOptions(devices) : undefined,
      )
      if (enable && devices.microphoneGain !== 100) {
        await applyMicrophoneGain(
          publication?.audioTrack,
          devices.microphoneGain,
        )
      }
      revision += 1
    } catch (error) {
      controlError =
        error instanceof Error ? error.message : 'Microphone access denied'
    } finally {
      controlBusy = false
    }
  }
  async function toggleScreen() {
    if (!call) return
    controlBusy = true
    controlError = ''
    try {
      const enable = !call.localParticipant.isScreenShareEnabled
      const remoteSharer = enable
        ? Array.from(call.remoteParticipants.values()).find((participant) =>
            participant.getTrackPublication(Track.Source.ScreenShare),
          )
        : undefined
      if (remoteSharer) {
        controlError = `${remoteSharer.name || 'Another participant'} is already sharing their screen`
        return
      }
      const quality = settingsQuery.data?.video_quality ?? 'high'
      if (!screenPublisher) throw new Error('Video negotiation is not ready')
      if (enable) await screenPublisher.start(quality !== 'low')
      else await screenPublisher.stop()
      revision += 1
    } catch (error) {
      controlError =
        error instanceof Error
          ? error.message
          : 'Screen sharing is unavailable in this browser'
    } finally {
      controlBusy = false
    }
  }
  async function copyInvite() {
    try {
      await navigator.clipboard.writeText(inviteURL(inviteCode))
      copied = true
      window.setTimeout(() => (copied = false), 1800)
    } catch {
      controlError = 'Could not copy link'
    }
  }
  async function toggleFullscreen() {
    controlError = ''
    try {
      if (document.fullscreenElement) await document.exitFullscreen()
      else await stageRef?.requestFullscreen()
    } catch {
      controlError = 'Fullscreen is unavailable in this browser'
    }
  }
  function openChat() {
    sidePanel = 'chat'
    if (window.matchMedia('(max-width: 760px)').matches) {
      window.requestAnimationFrame(() =>
        sidePanelRef?.scrollIntoView({ behavior: 'smooth', block: 'start' }),
      )
    }
  }
  function submitMessage(event: SubmitEvent) {
    event.preventDefault()
    const body = messageBody.trim()
    if (!body || sendMessage.isPending) return
    sendMessage.mutate(body)
  }
  async function leave() {
    const room = activeCall
    activeCall = null
    if (room) {
      stopLocalMedia(room)
      await room.disconnect()
    }
    const callID = directCallID
    directCallID = null
    if (callID) {
      await api<void>(`/api/calls/${callID}/end`, { method: 'POST' }).catch(
        () => undefined,
      )
    }
    call = null
    await navigate({ to: '/' })
  }

  function microphoneCaptureOptions(settings: LocalDeviceSettings) {
    return {
      deviceId: settings.audioInputId
        ? { exact: settings.audioInputId }
        : undefined,
      echoCancellation: true,
      autoGainControl: true,
      noiseSuppression: settings.noiseSuppression,
      voiceIsolation: settings.noiseSuppression,
    }
  }
  function stopLocalMedia(room: Room) {
    if (room === call || room === activeCall || !activeCall) {
      screenPublisher?.dispose()
      screenPublisher = null
    }
    room.localParticipant
      .getTrackPublications()
      .forEach((publication) => publication.track?.stop())
  }
  function endDirectCall(id: string | null) {
    if (!id) return
    void fetch(`/api/calls/${id}/end`, {
      method: 'POST',
      credentials: 'same-origin',
      keepalive: true,
    })
  }
</script>

{#if userLoading}
  <main class="page-shell py-20">
    <div class="skeleton h-[65dvh]"></div>
  </main>
{:else if !user}
  <Navigate to={'/login?next=' + encodeURIComponent('/r/' + inviteCode)} />
{:else if user.must_change_password}
  <Navigate to="/first-password" />
{:else if roomQuery.isLoading}
  <main class="page-shell py-20">
    <div class="skeleton h-[65dvh]"></div>
  </main>
{:else if roomQuery.error || !roomQuery.data}
  <main class="page-shell grid min-h-[72dvh] place-items-center text-center">
    <div>
      <div class="font-mono text-sm text-accent">ROOM NOT FOUND</div>
      <h1 class="font-display mt-4 text-5xl font-semibold">
        Room unavailable
      </h1>
      <p class="mt-3 text-ink-muted">
        Check the link or ask for a new invitation.
      </p>
      <a href="/" class="button-primary mt-7">Go home</a>
    </div>
  </main>
{:else}
  <main class="room-page">
    <div bind:this={audioHost} class="hidden" aria-hidden="true"></div>
    <div class="room-topbar">
      <a href="/" class="brand room-brand"
        ><span class="brand-dot"></span><span>Mowa</span></a
      >
      <h1>{roomQuery.data.name}</h1>
      <code>{inviteCode}</code>
      <button class="button-secondary compact" onclick={copyInvite}>
        {#if copied}
          <Check size={15} />
        {:else}
          <Copy size={15} />
        {/if}
        {copied ? 'Copied' : 'Copy link'}</button
      >
      <span class="room-topbar-spacer"></span>
      <span class={connected ? 'broadcast-status online' : 'broadcast-status'}
        ><i></i>{connected ? 'connected' : 'room ready'}</span
      >
    </div>

    {#if !connected}
      <section class="join-stage">
        <div class="join-visual" aria-hidden="true">
          <div class="avatar-preview">
            {initials(user.display_name)}
          </div>
          <div class="ring ring-one"></div>
          <div class="ring ring-two"></div>
        </div>
        <div class="max-w-lg text-center">
          <h2
            class="font-display text-4xl font-semibold tracking-[-0.045em] sm:text-5xl"
          >
            Ready to join?
          </h2>
          <p class="mt-4 leading-relaxed text-ink-muted">
            Your browser will request microphone access.
          </p>
          {#if controlError}
            <p class="error-note mt-5" role="alert">
              {controlError}
            </p>
          {/if}
          <button
            class="button-primary mt-7"
            onclick={() => join.mutate()}
            disabled={join.isPending}
          >
            <Radio size={19} />
            {join.isPending ? 'Connecting…' : 'Join call'}
          </button>
        </div>
      </section>
    {:else}
      <div class="call-grid">
        <section bind:this={stageRef} class="stage-panel">
          {#if screenPublication}
            <ScreenTrack publication={screenPublication} {revision} />
            <button
              class="fullscreen-button"
              onclick={toggleFullscreen}
              aria-label={fullscreen
                ? 'Exit fullscreen'
                : 'View screen share in fullscreen'}
              title={fullscreen
                ? 'Exit fullscreen'
                : 'Fullscreen'}
            >
              {#if fullscreen}
                <Minimize2 size={18} />
              {:else}
                <Maximize2 size={18} />
              {/if}
              <span>{fullscreen ? 'Exit fullscreen' : 'Fullscreen'}</span>
            </button>
          {:else}
            <div class="empty-stage">
              <h2>No one is sharing their screen</h2>
              <p>Use the screen button below to share yours</p>
            </div>
          {/if}
        </section>

        <aside bind:this={sidePanelRef} class="room-side-panel">
          <div
            class="room-side-tabs"
            role="tablist"
            aria-label="Room panel"
          >
            <button
              class={sidePanel === 'participants' ? 'active' : ''}
              onclick={() => (sidePanel = 'participants')}
              role="tab"
              aria-selected={sidePanel === 'participants'}
              ><Users size={16} />Participants
              <span>{participants.length}</span></button
            >
            <button
              class={sidePanel === 'chat' ? 'active' : ''}
              onclick={() => (sidePanel = 'chat')}
              role="tab"
              aria-selected={sidePanel === 'chat'}
              ><MessageSquare size={16} />Chat</button
            >
          </div>
          {#if sidePanel === 'participants'}
            <div class="participants-panel" role="tabpanel">
              <div class="participants-list">
                {#each participants ?? [] as participant (participant.identity)}
                  <ParticipantRow
                    {participant}
                    local={participant.identity ===
                      call?.localParticipant.identity}
                    {revision}
                  />
                {/each}
              </div>
            </div>
          {:else}
            <div class="chat-panel" role="tabpanel" aria-label="Room chat">
              <div class="chat-messages" aria-live="polite">
                {#if messagesQuery.isLoading}
                  <p class="chat-state">Loading messages…</p>
                {/if}
                {#if messagesQuery.error}
                  <p class="chat-state error">Could not load messages</p>
                {/if}
                {#if !messagesQuery.isLoading && !messagesQuery.error && messagesQuery.data?.length === 0}
                  <p class="chat-state">No messages yet.</p>
                {/if}
                {#each messagesQuery.data ?? [] as message (message.id)}
                  <ChatMessage {message} own={message.author.id === user.id} />
                {/each}
                <div bind:this={messagesEndRef}></div>
              </div>
              <form class="chat-composer" onsubmit={submitMessage}>
                <textarea
                  value={messageBody}
                  oninput={(event) => (messageBody = event.currentTarget.value)}
                  onkeydown={(event) => {
                    if (
                      event.key === 'Enter' &&
                      !event.shiftKey &&
                      !event.isComposing
                    ) {
                      event.preventDefault()
                      event.currentTarget.form?.requestSubmit()
                    }
                  }}
                  maxlength={2000}
                  rows={2}
                  placeholder="Message…"
                  aria-label="Message"></textarea>
                <button
                  type="submit"
                  disabled={!messageBody.trim() || sendMessage.isPending}
                  aria-label="Send message"><Send size={18} /></button
                >
                {#if messageBody.length > 1800}
                  <small>{messageBody.length}/2000</small>
                {/if}
              </form>
              {#if sendMessage.error}
                <p class="chat-send-error" role="alert">
                  {sendMessage.error.message}
                </p>
              {/if}
            </div>
          {/if}
        </aside>
      </div>
    {/if}

    {#if connected}
      <div class="control-dock" aria-label="Call controls">
        <button
          class={`call-control ${!micEnabled ? 'danger' : ''}`}
          onclick={toggleMic}
          disabled={controlBusy}
          aria-label={micEnabled ? 'Mute microphone' : 'Unmute microphone'}
          title={micEnabled ? 'Mute microphone' : 'Unmute microphone'}
        >
          {#if micEnabled}
            <Mic size={21} />
          {:else}
            <MicOff size={21} />
          {/if}
        </button>
        <button
          class={`call-control wide ${screenEnabled ? 'active' : ''}`}
          onclick={toggleScreen}
          disabled={controlBusy || remoteScreenShareActive}
          aria-label={screenEnabled
            ? 'Stop sharing'
            : remoteScreenShareActive
              ? 'Another participant is already sharing'
              : 'Share screen'}
          title={remoteScreenShareActive
            ? `${activeScreenShare?.participant.name || 'Another participant'} is already sharing their screen`
            : undefined}
        >
          <MonitorUp size={21} /><span
            >{screenEnabled
              ? 'Stop'
              : remoteScreenShareActive
                ? 'Busy'
                : 'Screen'}</span
          >
        </button>
        <button
          class="call-control"
          onclick={() => (callSettingsOpen = true)}
          aria-label="Call settings"
          title="Call settings"
        >
          <Settings size={21} />
        </button>
        <button
          class={`call-control ${sidePanel === 'chat' ? 'active' : ''}`}
          onclick={openChat}
          aria-label="Open chat"
          title="Chat"
        >
          <MessageSquare size={21} />
        </button>
        <button
          class="call-control danger"
          onclick={leave}
          aria-label="Leave room"
          title="Sign out"
        >
          <PhoneOff size={21} />
        </button>
      </div>
    {/if}
    {#if connected && controlError}
      <div class="room-error" role="alert">
        {controlError}
      </div>
    {/if}
    {#if callSettingsOpen}
      <CallSettingsModal
        room={call}
        settings={settingsQuery.data}
        onClose={() => (callSettingsOpen = false)}
        onSettingsSaved={(next) => {
          queryClient.setQueryData(['account-settings'], next)
        }}
      />
    {/if}
  </main>
{/if}
