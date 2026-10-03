<script lang="ts">
  import { untrack } from 'svelte'

  import { createMutation } from '@tanstack/svelte-query'

  import { Check } from '@lucide/svelte'
  import { api, type AccountSettings } from '../api'

  let {
    value,
    onSaved,
  }: {
    value: AccountSettings['video_quality']
    onSaved: (settings: AccountSettings) => void
  } = $props()
  let quality = $state(untrack(() => value))
  const mutation = createMutation(() => ({
    mutationFn: () =>
      api<AccountSettings>('/api/account/settings', {
        method: 'PUT',
        body: JSON.stringify({ video_quality: quality }),
      }),
    onSuccess: onSaved,
  }))
</script>

<form
  class="settings-fields"
  onsubmit={(event) => {
    event.preventDefault()
    mutation.mutate()
  }}
>
  <label class="field-label"
    >Качество<select
      class="text-input"
      value={quality}
      oninput={(event) =>
        (quality = event.currentTarget
          .value as AccountSettings['video_quality'])}
      ><option value="low">720p · 30 кадров/с</option><option value="high"
        >1080p · 30 кадров/с</option
      ></select
    ></label
  >
  <button
    class="button-primary compact settings-save"
    disabled={mutation.isPending}
  >
    {#if mutation.isSuccess}
      <Check size={16} /> Сохранено{:else}Сохранить{/if}
  </button>
</form>
