<script lang="ts">
  import { createMutation, useQueryClient } from '@tanstack/svelte-query'
  import { api, type User } from '../api'
  import { focusOnMount } from '../focus'
  import { navigate } from '../navigation.svelte'
  let { user }: { user: User } = $props()
  let confirming = $state(false)
  let confirmation = $state('')
  const queryClient = useQueryClient()
  const deletion = createMutation(() => ({
    mutationFn: () => api<void>('/api/account', { method: 'DELETE', body: JSON.stringify({ username: confirmation }) }),
    onSuccess: async () => {
      await queryClient.cancelQueries()
      queryClient.clear()
      queryClient.setQueryData(['me'], null)
      navigate({ to: '/login', replace: true })
    },
  }))
</script>

<section class="settings-card delete-account">
  <h2>Delete account</h2>
  <p class="settings-hint">Permanently delete your account, messages, friendships and rooms you created. You will be signed out on all devices. This cannot be undone.</p>
  {#if confirming}
    <form class="settings-fields" onsubmit={(event) => { event.preventDefault(); if (confirmation === user.username && !deletion.isPending) deletion.mutate() }}>
      <label class="field-label">Type <strong>{user.username}</strong> to confirm
        <input use:focusOnMount class="text-input" aria-label="Confirm your username" bind:value={confirmation} autocomplete="off" autocapitalize="none" spellcheck={false} required disabled={deletion.isPending} />
      </label>
      {#if deletion.error}<p class="error-note" role="alert">{deletion.error.message}</p>{/if}
      <div class="delete-actions">
        <button class="button-secondary compact" type="button" disabled={deletion.isPending} onclick={() => { confirming = false; confirmation = ''; deletion.reset() }}>Cancel</button>
        <button class="button-primary compact delete-button" disabled={confirmation !== user.username || deletion.isPending}>{deletion.isPending ? 'Deleting…' : 'Permanently delete account'}</button>
      </div>
    </form>
  {:else}
    <button class="button-secondary compact delete-outline" onclick={() => { confirming = true }}>Delete account</button>
  {/if}
</section>

<style>
  .delete-account { border-color: color-mix(in srgb, #d94e46 35%, var(--border)); }
  .delete-account > p { margin-bottom: 18px; }
  .delete-outline { color: #d94e46; }
  .delete-button { background: #bd352e; color: white; }
  .delete-button:hover:not(:disabled) { background: #a92f29; }
  .delete-actions { display: flex; flex-wrap: wrap; gap: 12px; justify-content: flex-end; }
</style>
