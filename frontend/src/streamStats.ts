export interface ReceiverSample {
  streamId?: string
  timestamp: number
  bytesReceived?: number
  framesDecoded?: number
  framesDropped?: number
  frameWidth?: number
  frameHeight?: number
  mimeType?: string
}

export function streamMetrics(current: ReceiverSample, previous?: ReceiverSample) {
  const valid = (n: number | undefined): n is number =>
    n !== undefined && Number.isFinite(n) && n >= 0
  const elapsed = previous ? (current.timestamp - previous.timestamp) / 1000 : 0
  const sameStream = Boolean(current.streamId) && previous?.streamId === current.streamId && Number.isFinite(elapsed) && elapsed > 0
  const rate = (next: number | undefined, before: number | undefined) =>
    sameStream && valid(next) && valid(before) && next >= before
      ? (next - before) / elapsed
      : null
  const bytesPerSecond = rate(current.bytesReceived, previous?.bytesReceived)
  const codec = current.mimeType?.replace(/^video\//i, '').toUpperCase()
  return {
    codec: codec === 'H264' ? 'H.264' : codec === 'H265' ? 'HEVC' : codec,
    resolution:
      valid(current.frameWidth) && current.frameWidth > 0 &&
      valid(current.frameHeight) && current.frameHeight > 0
        ? `${current.frameWidth} × ${current.frameHeight}` : null,
    fps: rate(current.framesDecoded, previous?.framesDecoded),
    mbps: bytesPerSecond === null ? null : bytesPerSecond * 8 / 1_000_000,
    dropped: valid(current.framesDropped) ? current.framesDropped : null,
  }
}
