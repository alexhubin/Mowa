<script lang="ts">
  import {
    createQuery,
    createMutation,
    useQueryClient,
  } from '@tanstack/svelte-query'

  import { KeyRound, Plus, Trash2 } from '@lucide/svelte'
  import { api } from '../api'

  import { passkeysSupported, registerPasskey, type Passkey } from '../passkeys'

  const queryClient = useQueryClient()
  let name = $state('My passkey')
  const passkeys = createQuery(() => ({
    queryKey: ['passkeys'],
    queryFn: () => api<Passkey[]>('/api/account/passkeys'),
  }))
  const create = createMutation(() => ({
    mutationFn: () => registerPasskey(name),
    onSuccess: (passkey) => {
      queryClient.setQueryData<Passkey[]>(['passkeys'], (current = []) => [
        passkey,
        ...current,
      ])
      name = 'My passkey'
    },
  }))
  const remove = createMutation(() => ({
    mutationFn: (id: string) =>
      api<void>(`/api/account/passkeys/${id}`, { method: 'DELETE' }),
    onSuccess: (_, id) =>
      queryClient.setQueryData<Passkey[]>(['passkeys'], (current = []) =>
        current.filter((item) => item.id !== id),
      ),
  }))
  const supported = $derived(passkeysSupported())

  function formatPasskeyDate(value: string) {
    return new Intl.DateTimeFormat('en-US', {
      dateStyle: 'medium',
      timeStyle: 'short',
    }).format(new Date(value))
  }
</script>

<section class="settings-card">
  <h2>Passkeys</h2>
  <div class="passkey-intro">
    <KeyRound size={20} />
    <p>
      Use Touch ID, Face ID, Windows Hello or a security
      key.
    </p>
  </div>
  {#if supported}
    <form
      class="passkey-create"
      onsubmit={(event) => {
        event.preventDefault()
        create.mutate()
      }}
    >
      <input
        class="text-input"
        value={name}
        oninput={(event) => (name = event.currentTarget.value)}
        maxlength={50}
        required
        aria-label="Passkey name"
        placeholder="e.g. MacBook"
      />
      <button
        class="button-primary compact"
        disabled={create.isPending || !name.trim()}
        ><Plus size={16} />
        {create.isPending ? 'Confirm…' : 'Add'}</button
      >
    </form>
    {#if passkeys.error || create.error || remove.error}
      <p class="error-note">
        {passkeys.error?.message ||
          create.error?.message ||
          remove.error?.message}
      </p>
    {/if}
    <div class="passkey-list">
      {#if passkeys.isLoading}
        <div class="skeleton h-16"></div>
      {/if}
      {#each passkeys.data ?? [] as passkey (passkey.id)}
        <div class="passkey-row">
          <span class="passkey-icon"><KeyRound size={17} /></span>
          <span
            ><strong>{passkey.name}</strong><small
              >{passkey.last_used_at
                ? `Last used: ${formatPasskeyDate(passkey.last_used_at)}`
                : `Added: ${formatPasskeyDate(passkey.created_at)}`}</small
            ></span
          >
          <button
            type="button"
            class="mini-action"
            aria-label={`Remove ${passkey.name}`}
            title="Remove passkey"
            disabled={remove.isPending}
            onclick={() => {
              if (window.confirm(`Remove passkey «${passkey.name}»?`))
                remove.mutate(passkey.id)
            }}><Trash2 size={16} /></button
          >
        </div>
      {/each}
      {#if !passkeys.isLoading && passkeys.data?.length === 0}
        <p class="settings-hint">
          No passkeys yet. You can still sign in
          with your password.
        </p>
      {/if}
    </div>
  {:else}
    <p class="settings-hint">
      Passkeys require a supported browser and a secure
      HTTPS connection.
    </p>
  {/if}
</section>
