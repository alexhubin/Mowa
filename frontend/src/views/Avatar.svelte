<script lang="ts">
  import { initials } from '../utils'

  let { name, online }: { name: string; online?: boolean } = $props()
</script>

<span class="participant-avatar"
  >{initials(name)}{#if online !== undefined}
    <i class={online ? 'online' : ''}></i>
  {/if}
</span>
