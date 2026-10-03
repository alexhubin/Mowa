<script lang="ts">
  import { focusOnMount } from '../focus'

  import {
    createQuery,
    createMutation,
    useQueryClient,
  } from '@tanstack/svelte-query'

  import { Send, X } from '@lucide/svelte'
  import { api, type DirectMessage, type FriendUser } from '../api'

  import Avatar from './Avatar.svelte'
  import UserLabel from './UserLabel.svelte'

  let {
    userID,
    friend,
    onClose,
  }: { userID: string; friend: FriendUser; onClose: () => void } = $props()
  const queryClient = useQueryClient()
  let messagesEnd: HTMLDivElement | null = $state.raw(null)
  let body = $state('')
  const messages = createQuery(() => ({
    queryKey: ['direct-messages', friend.id],
    queryFn: () => api<DirectMessage[]>(`/api/direct-messages/${friend.id}`),
  }))
  const sendMessage = createMutation(() => ({
    mutationFn: (message: string) =>
      api<DirectMessage>(`/api/direct-messages/${friend.id}`, {
        method: 'POST',
        body: JSON.stringify({ body: message }),
      }),
    onSuccess: async () => {
      body = ''
      await queryClient.invalidateQueries({
        queryKey: ['direct-messages', friend.id],
      })
    },
  }))
  $effect(() => {
    const events = new EventSource(`/api/direct-messages/${friend.id}/events`)
    const refresh = () =>
      void queryClient.invalidateQueries({
        queryKey: ['direct-messages', friend.id],
      })
    events.addEventListener('messages', refresh)
    return () => {
      events.removeEventListener('messages', refresh)
      events.close()
    }
  })
  $effect(() => {
    void messages.data
    messagesEnd?.scrollIntoView({ block: 'end' })
  })
  function submitMessage(event: SubmitEvent) {
    event.preventDefault()
    const message = body.trim()
    if (!message || sendMessage.isPending) return
    sendMessage.mutate(message)
  }

  const messageTime = new Intl.DateTimeFormat('ru-RU', {
    hour: '2-digit',
    minute: '2-digit',
  })
</script>

<div
  class="modal-backdrop"
  role="presentation"
  onmousedown={(event) => {
    if (event.target === event.currentTarget) onClose()
  }}
>
  <div
    class="direct-chat-modal"
    role="dialog"
    aria-modal="true"
    aria-label={`Диалог с ${friend.display_name}`}
  >
    <header class="direct-chat-header">
      <Avatar name={friend.display_name} online={friend.online} />
      <UserLabel
        user={friend}
        detail={friend.online ? 'в сети' : 'сообщение будет доставлено позже'}
      />
      <button type="button" onclick={onClose} aria-label="Закрыть диалог"
        ><X size={19} /></button
      >
    </header>
    <div class="chat-messages" aria-live="polite">
      {#if messages.isLoading}
        <p class="chat-state">Загружаем сообщения…</p>
      {/if}
      {#if messages.error}
        <p class="chat-state error">Не удалось загрузить сообщения</p>
      {/if}
      {#if !messages.isLoading && !messages.error && messages.data?.length === 0}
        <p class="chat-state">Здесь пока тихо. Напишите первым.</p>
      {/if}
      {#each messages.data ?? [] as message (message.id)}
        <article
          class={`chat-message ${message.author.id === userID ? 'own' : ''}`}
        >
          <div class="chat-message-heading">
            <strong
              >{message.author.id === userID
                ? 'Вы'
                : message.author.display_name}</strong
            ><time datetime={message.created_at}
              >{messageTime.format(new Date(message.created_at))}</time
            >
          </div>
          <p>{message.body}</p>
        </article>
      {/each}
      <div bind:this={messagesEnd}></div>
    </div>
    <form class="chat-composer" onsubmit={submitMessage}>
      <textarea
        use:focusOnMount
        value={body}
        oninput={(event) => (body = event.currentTarget.value)}
        onkeydown={(event) => {
          if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
            event.preventDefault()
            event.currentTarget.form?.requestSubmit()
          }
        }}
        maxlength={2000}
        rows={2}
        placeholder={friend.online ? 'Сообщение…' : 'Сообщение офлайн-другу…'}
        aria-label="Сообщение"></textarea>
      <button
        type="submit"
        disabled={!body.trim() || sendMessage.isPending}
        aria-label="Отправить сообщение"><Send size={18} /></button
      >
      {#if body.length > 1800}
        <small>{body.length}/2000</small>
      {/if}
    </form>
    {#if sendMessage.error}
      <p class="chat-send-error" role="alert">
        {sendMessage.error.message}
      </p>
    {/if}
  </div>
</div>
