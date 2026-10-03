import { supportsAV1, supportsVP9 } from 'livekit-client'

// Shared with Mowa-desktop/src/video_policy.rs. Missing attributes mean an old client.
export const RECEIVE_CODECS = 'mowa.video.receive.v1'
export const codecOrder = ['av1', 'vp9', 'h264'] as const
export type ScreenCodec = typeof codecOrder[number]
export function parseCodecs(value: string | undefined): ScreenCodec[] {
  if (value === undefined) return ['h264']
  const values = value.split(',')
  return codecOrder.filter(codec => values.includes(codec))
}
export function chooseCodec(send: readonly ScreenCodec[], receivers: readonly (readonly ScreenCodec[])[]): ScreenCodec | undefined {
  return codecOrder.find(codec => send.includes(codec) && receivers.every(caps => caps.includes(codec)))
}
export function bitrate(codec: ScreenCodec, high: boolean): number {
  return { av1: 8_000_000, vp9: 10_000_000, h264: 14_000_000 }[codec] / (high ? 1 : 2)
}
export function fromCapabilities(codecs: readonly { mimeType: string; sdpFmtpLine?: string }[]): ScreenCodec[] {
  return codecOrder.filter(codec => codecs.some(c => {
    if (c.mimeType.toLowerCase() !== `video/${codec}`) return false
    const fmtp = c.sdpFmtpLine ?? ''
    // Our encoders use 8-bit profile 0 for AV1/VP9 and packetized baseline H.264.
    if (codec === 'vp9') return !/profile-id=([1-9])/.test(fmtp)
    if (codec === 'av1') return !/profile=([1-9])/.test(fmtp)
    return /packetization-mode=1/.test(fmtp) && /profile-level-id=42[0-9a-f]{4}/i.test(fmtp)
  }))
}
export function browserCodecs() {
  const receive = fromCapabilities(RTCRtpReceiver.getCapabilities('video')?.codecs ?? [])
  const send = fromCapabilities(RTCRtpSender.getCapabilities('video')?.codecs ?? []).filter(codec =>
    codec === 'av1' ? supportsAV1() : codec === 'vp9' ? supportsVP9() : true,
  )
  return { receive, send }
}
