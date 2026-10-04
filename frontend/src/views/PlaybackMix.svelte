<script lang="ts">
  import { loadMix, saveMix, type PlaybackMix } from '../playbackMix'
  let mix = $state(loadMix())
  function change(channel: keyof PlaybackMix, value: number) {
    mix = { ...mix, [channel]: value }
    saveMix(mix)
  }
</script>
<div class="space-y-4">
  <p class="text-sm text-muted">Playback volume · only changes what you hear</p>
  {#each [['voice', 'Voice'], ['stream', 'Stream audio']] as [channel, label] (channel)}
    <label class="range-setting">
      <span>{label}<strong>{mix[channel as keyof PlaybackMix]}%</strong></span>
      <input type="range" min="0" max="100" step="1" value={mix[channel as keyof PlaybackMix]}
        oninput={(event) => change(channel as keyof PlaybackMix, Number(event.currentTarget.value))} />
    </label>
  {/each}
</div>
