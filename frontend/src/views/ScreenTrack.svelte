<script lang="ts">
  import type { TrackPublication } from 'livekit-client'
  let {
    publication,
    revision,
  }: { publication: TrackPublication; revision: number } = $props()
  let video: HTMLVideoElement | null = $state.raw(null)
  const track = $derived.by(() => {
    void revision
    return publication.track
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
