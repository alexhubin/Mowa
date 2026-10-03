<script lang="ts">
  import { navigate } from '../navigation.svelte'
  let { to }: { to: string } = $props()
  $effect(() => {
    navigate({ to, replace: true })
  })
</script>
