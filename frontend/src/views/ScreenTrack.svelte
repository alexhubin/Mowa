<script lang="ts">
  import type { TrackPublication, RemoteVideoTrack } from 'livekit-client'
  import { streamMetrics, type ReceiverSample } from '../streamStats'
  let {
    publication,
    revision,
  }: { publication: TrackPublication; revision: number } = $props()
  let video: HTMLVideoElement | null = $state.raw(null)
  const track = $derived.by(() => {
    void revision
    return publication.track
  })
  let showStats = $state(false)
  let metrics = $state<ReturnType<typeof streamMetrics> | null>(null)
  let statsUnavailable = $state(false)
  const incoming = $derived(Boolean(track && !track.isLocal))
  $effect(() => {
    const remote = track
    if (!showStats || !remote || remote.isLocal) return
    let cancelled = false
    let timer: ReturnType<typeof setTimeout> | undefined
    let previous: ReceiverSample | undefined
    metrics = null
    statsUnavailable = false
    async function sample() {
      try {
        const result = await (remote as RemoteVideoTrack).getReceiverStats()
        if (cancelled) return
        if (result) {
          metrics = streamMetrics(result, previous)
          previous = result
          statsUnavailable = false
        } else {
          metrics = null
          previous = undefined
          statsUnavailable = true
        }
      } catch {
        if (cancelled) return
        metrics = null
        previous = undefined
        statsUnavailable = true
      }
      if (!cancelled) timer = setTimeout(() => void sample(), 1000)
    }
    void sample()
    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  })
  $effect(() => {
    const element = video
    const attachedTrack = track
    if (!element || !attachedTrack) return
    attachedTrack.attach(element)
    return () => {
      attachedTrack.detach(element)
    }
  })
</script>

<video bind:this={video} class="screen-video" autoplay playsinline></video>

{#if incoming}
  <div class="stream-stats">
    <button class="stats-toggle" aria-expanded={showStats} onclick={() => (showStats = !showStats)}>
      {showStats ? 'Hide stats' : 'Stats'}
    </button>
    {#if showStats}
      <section class="stats-body" aria-label="Incoming video statistics">
        <strong>Incoming video</strong>
        {#if statsUnavailable}
          <p>Statistics unavailable in this browser.</p>
        {:else if !metrics}
          <p>Waiting for video statistics…</p>
        {:else}
          <dl>
            <div><dt>Codec</dt><dd>{metrics.codec ?? '—'}</dd></div>
            <div><dt>Resolution</dt><dd>{metrics.resolution ?? '—'}</dd></div>
            <div><dt>Decoded FPS</dt><dd>{metrics.fps?.toFixed(1) ?? '—'}</dd></div>
            <div><dt>Video bitrate</dt><dd>{metrics.mbps === null ? '—' : `${metrics.mbps.toFixed(2)} Mbps`}</dd></div>
            <div><dt>Dropped frames</dt><dd>{metrics.dropped ?? '—'}</dd></div>
          </dl>
          <p>Rates update every second. Drops are cumulative for this stream. Decoded FPS is not display FPS.</p>
        {/if}
      </section>
    {/if}
  </div>
{/if}

<style>
  .stream-stats { position: absolute; z-index: 4; top: 12px; left: 12px; max-width: calc(100% - 24px); color: #edf8f2; font-size: 12px; }
  .stats-toggle { border: 1px solid #688477; border-radius: 8px; padding: 7px 12px; background: #10261eee; color: inherit; cursor: pointer; }
  .stats-toggle:focus-visible { outline: 2px solid #5fd8a4; outline-offset: 3px; }
  .stats-body { margin-top: 6px; width: 255px; max-width: 100%; border: 1px solid #385246; border-radius: 10px; padding: 12px; background: #0b1a15f5; }
  strong { font-size: 12px; }
  dl { margin: 10px 0; }
  dl div { display: flex; justify-content: space-between; gap: 16px; margin-top: 6px; }
  dt { color: #b9cfc3; }
  dd { margin: 0; font-variant-numeric: tabular-nums; }
  p { margin: 8px 0 0; font-size: 11px; line-height: 1.5; color: #b9cfc3; }
</style>
