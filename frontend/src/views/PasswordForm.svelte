<script lang="ts">
  import { createMutation } from '@tanstack/svelte-query'

  import { Check } from '@lucide/svelte'
  import { api } from '../api'

  let currentPassword = $state('')
  let newPassword = $state('')
  const mutation = createMutation(() => ({
    mutationFn: () =>
      api<void>('/api/account/password', {
        method: 'PUT',
        body: JSON.stringify({
          current_password: currentPassword,
          new_password: newPassword,
        }),
      }),
    onSuccess: () => {
      currentPassword = ''
      newPassword = ''
    },
  }))
  function submit(event: SubmitEvent) {
    event.preventDefault()
    mutation.mutate()
  }
</script>

<form class="settings-fields" onsubmit={submit}>
  <h3>Change password</h3>
  <label class="field-label"
    >Current password<input
      class="text-input"
      type="password"
      autocomplete="current-password"
      value={currentPassword}
      oninput={(event) => (currentPassword = event.currentTarget.value)}
      required
    /></label
  >
  <label class="field-label"
    >New password<input
      class="text-input"
      type="password"
      autocomplete="new-password"
      minlength={8}
      maxlength={128}
      value={newPassword}
      oninput={(event) => (newPassword = event.currentTarget.value)}
      required
    /></label
  >
  {#if mutation.error}
    <p class="error-note">{mutation.error.message}</p>
  {/if}
  <button
    class="button-secondary compact settings-save"
    disabled={mutation.isPending}
  >
    {#if mutation.isSuccess}
      <Check size={16} /> Password changed{:else}Change password{/if}
  </button>
</form>
