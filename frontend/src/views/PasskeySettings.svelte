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
  let name = $state('Мой passkey')
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
      name = 'Мой passkey'
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
    return new Intl.DateTimeFormat('ru-RU', {
      dateStyle: 'medium',
      timeStyle: 'short',
    }).format(new Date(value))
  }
</script>

<section class="settings-card">
  <h2>Вход по passkey</h2>
  <div class="passkey-intro">
    <KeyRound size={20} />
    <p>
      Входите без пароля через Touch ID, Face ID, Windows Hello или ключ
      безопасности.
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
        aria-label="Название passkey"
        placeholder="Например, MacBook"
      />
      <button
        class="button-primary compact"
        disabled={create.isPending || !name.trim()}
        ><Plus size={16} />
        {create.isPending ? 'Подтвердите…' : 'Добавить'}</button
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
                ? `Последний вход: ${formatPasskeyDate(passkey.last_used_at)}`
                : `Добавлен: ${formatPasskeyDate(passkey.created_at)}`}</small
            ></span
          >
          <button
            type="button"
            class="mini-action"
            aria-label={`Удалить ${passkey.name}`}
            title="Удалить passkey"
            disabled={remove.isPending}
            onclick={() => {
              if (window.confirm(`Удалить passkey «${passkey.name}»?`))
                remove.mutate(passkey.id)
            }}><Trash2 size={16} /></button
          >
        </div>
      {/each}
      {#if !passkeys.isLoading && passkeys.data?.length === 0}
        <p class="settings-hint">
          Passkey пока не добавлены. Пароль останется доступен как резервный
          способ входа.
        </p>
      {/if}
    </div>
  {:else}
    <p class="settings-hint">
      Passkey недоступен: нужен современный браузер и защищённое
      HTTPS-соединение.
    </p>
  {/if}
</section>
