<script lang="ts">
  import { MessageCircle, Phone, PhoneOff } from '@lucide/svelte'
  import { type FriendUser } from '../api'

  import Avatar from './Avatar.svelte'
  import UserLabel from './UserLabel.svelte'

  let {
    friend,
    onMessage,
    onCall,
    busy,
  }: {
    friend: FriendUser
    onMessage: () => void
    onCall: () => void
    busy: boolean
  } = $props()
</script>

<article class={`friend-row ${friend.online ? '' : 'offline-row'}`}>
  <Avatar name={friend.display_name} online={friend.online} /><UserLabel
    user={friend}
    detail={friend.online ? 'в сети' : 'не в сети'}
  />
  <div class="friend-actions">
    <button
      class="friend-message"
      onclick={onMessage}
      aria-label={`Написать ${friend.display_name}`}
      title="Написать"><MessageCircle size={16} /><span>Написать</span></button
    ><button
      class="friend-call"
      onclick={onCall}
      disabled={busy || !friend.online}
      aria-label={friend.online
        ? `Позвонить ${friend.display_name}`
        : `${friend.display_name} не в сети`}
      title={friend.online ? 'Позвонить' : 'Пользователь не в сети'}
    >
      {#if friend.online}
        <Phone size={16} />
      {:else}
        <PhoneOff size={16} />
      {/if}
      <span>{friend.online ? 'Позвонить' : 'Не в сети'}</span></button
    >
  </div>
</article>
