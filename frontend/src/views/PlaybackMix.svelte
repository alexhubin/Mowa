<script lang="ts">
  import { loadMix, saveMix, type PlaybackMix } from '../playbackMix'
  let mix = $state(loadMix())
  function change(channel: keyof PlaybackMix, value: number | boolean) {
    mix = { ...mix, [channel]: value }
    saveMix(mix)
  }
</script>
<div class="space-y-4">
  <p class="text-sm text-muted">What you hear · does not change your microphone</p>
  {#each [['voice', 'Other people’s voices'], ['stream', 'Streamer’s computer audio']] as [channel, label] (channel)}
    <label class="range-setting">
      <span>{label}<strong>{mix[channel as keyof PlaybackMix]}%</strong></span>
      <input type="range" min="0" max="200" step="1" value={mix[channel as keyof PlaybackMix]}
        oninput={(event) => change(channel as keyof PlaybackMix, Number(event.currentTarget.value))} />
      <span class="text-xs text-muted"><span>0% · Muted</span><span>100% · Original</span><span>200%</span></span>
    </label>
  {/each}
  <label class="flex items-center justify-between gap-4">
    <span>Lower stream audio while its author speaks</span>
    <input type="checkbox" checked={mix.ducking} onchange={(event) => change('ducking', event.currentTarget.checked)} />
  </label>
  <p class="text-sm text-muted">Only their computer audio gets quieter. Their voice stays at the volume above.</p>
  {#if mix.ducking}
    <label class="range-setting">
      <span>Reduce computer audio by<strong>{mix.reduction}%</strong></span>
      <input type="range" min="0" max="100" step="1" value={mix.reduction} oninput={(event) => change('reduction', Number(event.currentTarget.value))} />
      <span class="text-xs text-muted"><span>0% · No reduction</span><span>100% · Mute while speaking</span></span>
    </label>
  {/if}
</div>
