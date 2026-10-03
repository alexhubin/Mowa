<script lang="ts">
  import { UserPlus } from '@lucide/svelte'
  import { type FriendUser } from '../api'

  import Avatar from './Avatar.svelte'
  import UserLabel from './UserLabel.svelte'

  let {
    person,
    onAdd,
    busy,
  }: { person: FriendUser; onAdd: () => void; busy: boolean } = $props()
  const available = $derived(person.relationship === 'none')
  const label = $derived(
    person.relationship === 'friends'
      ? 'Already friends'
      : person.relationship === 'request_sent'
        ? 'Request sent'
        : person.relationship === 'request_received'
          ? 'Respond to request'
          : 'Add',
  )
</script>

<article class="friend-row">
  <Avatar name={person.display_name} /><UserLabel user={person} />
  {#if available}
    <button class="button-secondary compact" onclick={onAdd} disabled={busy}
      ><UserPlus size={16} /> {label}</button
    >
  {:else}
    <span class="request-state">{label}</span>
  {/if}
</article>
