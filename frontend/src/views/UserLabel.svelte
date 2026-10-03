<script lang="ts">
  import { type FriendUser } from '../api'

  let { user, detail }: { user: FriendUser; detail?: string } = $props()
</script>

<span class="user-label"
  ><strong>{user.display_name}</strong><small
    >{user.username}{detail ? ` · ${detail}` : ''}</small
  ></span
>
