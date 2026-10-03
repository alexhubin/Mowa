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
    detail={friend.online ? 'online' : 'offline'}
  />
  <div class="friend-actions">
    <button
      class="friend-message"
      onclick={onMessage}
      aria-label={`Message ${friend.display_name}`}
      title="Message"><MessageCircle size={16} /><span>Message</span></button
    ><button
      class="friend-call"
      onclick={onCall}
      disabled={busy || !friend.online}
      aria-label={friend.online
        ? `Call ${friend.display_name}`
        : `${friend.display_name} offline`}
      title={friend.online ? 'Call' : 'User is offline'}
    >
      {#if friend.online}
        <Phone size={16} />
      {:else}
        <PhoneOff size={16} />
      {/if}
      <span>{friend.online ? 'Call' : 'Offline'}</span></button
    >
  </div>
</article>
