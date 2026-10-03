<script lang="ts">
  import { untrack } from 'svelte'

  import { createMutation } from '@tanstack/svelte-query'

  import { Check } from '@lucide/svelte'
  import { api, type User } from '../api'

  let { user, onSaved }: { user: User; onSaved: (user: User) => void } =
    $props()
  let username = $state(untrack(() => user.username))
  const mutation = createMutation(() => ({
    mutationFn: () =>
      api<User>('/api/account/profile', {
        method: 'PATCH',
        body: JSON.stringify({ username }),
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
    >Username<input
      class="text-input"
      value={username}
      oninput={(event) => (username = event.currentTarget.value.toLowerCase())}
      minlength={3}
      maxlength={32}
      pattern="[a-z0-9_]+"
      required
    /></label
  >
  <label class="field-label"
    >Email<input class="text-input" value={user.email} readonly /></label
  >
  {#if mutation.error}
    <p class="error-note">{mutation.error.message}</p>
  {/if}
  <button
    class="button-primary compact settings-save"
    disabled={mutation.isPending}
  >
    {#if mutation.isSuccess}
      <Check size={16} /> Saved{:else}Save{/if}
  </button>
</form>
