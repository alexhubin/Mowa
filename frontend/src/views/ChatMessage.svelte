<script lang="ts">
  import { type RoomMessage } from '../api'

  let { message, own }: { message: RoomMessage; own: boolean } = $props()

  const messageTime = new Intl.DateTimeFormat('en-US', {
    hour: '2-digit',
    minute: '2-digit',
  })
</script>

<article class={`chat-message ${own ? 'own' : ''}`}>
  <div class="chat-message-heading">
    <strong>{own ? 'You' : message.author.display_name}</strong><time
      datetime={message.created_at}
      >{messageTime.format(new Date(message.created_at))}</time
    >
  </div>
  <p>{message.body}</p>
</article>
