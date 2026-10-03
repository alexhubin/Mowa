<script lang="ts">
  import { Mic, MicOff } from '@lucide/svelte'
  import { Track, type Participant } from 'livekit-client'

  import { initials } from '../utils'

  let {
    participant,
    local,
    revision,
  }: { participant: Participant; local: boolean; revision: number } = $props()
  const mic = $derived.by(() => {
    void revision
    return participant.getTrackPublication(Track.Source.Microphone)
  })
  const muted = $derived.by(() => {
    void revision
    return !mic || mic.isMuted
  })

  const speaking = $derived.by(() => {
    void revision
    return participant.isSpeaking
  })
</script>

<div class={`participant-row ${speaking ? 'speaking' : ''}`}>
  <div class="participant-avatar">
    {initials(participant.name || participant.identity)}
  </div>
  <div class="min-w-0 flex-1">
    <div class="truncate text-sm font-semibold">
      {participant.name || 'Участник'}
      {#if local}
        <span class="font-normal text-ink-muted">(вы)</span>
      {/if}
    </div>
    <div class="mt-0.5 text-xs text-ink-muted">
      {speaking ? 'говорит' : muted ? 'микрофон выключен' : 'слушает'}
    </div>
  </div>
  <span class={muted ? 'mic-state muted' : 'mic-state'}>
    {#if muted}
      <MicOff size={14} />
    {:else}
      <Mic size={14} />
    {/if}
  </span>
</div>
