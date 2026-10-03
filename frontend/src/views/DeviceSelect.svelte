<script lang="ts">
  let {
    label,
    value,
    devices,
    onChange,
    disabled,
  }: {
    label: string
    value: string
    devices: MediaDeviceInfo[]
    onChange: (value: string) => void
    disabled?: boolean
  } = $props()
</script>

<label class="field-label"
  >{label}<select
    class="text-input"
    {value}
    oninput={(event) => onChange(event.currentTarget.value)}
    {disabled}
    ><option value="">System default</option>
    {#each devices ?? [] as device, index (device.deviceId || index)}
      <option value={device.deviceId}
        >{device.label || `${label} ${index + 1}`}</option
      >
    {/each}
  </select></label
>
