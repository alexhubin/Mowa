import { describe, expect, it } from 'vitest'
import { streamMetrics, type ReceiverSample } from './streamStats'
const initial: ReceiverSample = {
  streamId: 'video-1', timestamp: 1000, bytesReceived: 100, framesDecoded: 10,
  mimeType: 'video/AV1', frameWidth: 1920, frameHeight: 1080, framesDropped: 2,
}
describe('incoming stream metrics', () => {
  it('uses counter differences and real sample time, not advertised settings', () => {
    const result = streamMetrics({ ...initial, timestamp: 3000, bytesReceived: 2000100, framesDecoded: 130 }, initial)
    expect(result).toEqual({ codec: 'AV1', resolution: '1920 × 1080', fps: 60, mbps: 8, dropped: 2 })
  })
  it('does not invent rates for first, replaced, reset, or duplicate samples', () => {
    expect(streamMetrics(initial).fps).toBeNull()
    expect(streamMetrics({ ...initial, timestamp: 2000, streamId: 'video-2' }, initial).mbps).toBeNull()
    expect(streamMetrics({ ...initial, timestamp: 2000, framesDecoded: 0, bytesReceived: 0 }, initial).fps).toBeNull()
    expect(streamMetrics(initial, initial).mbps).toBeNull()
  })
  it('shows a stalled stream as zero rather than its previous frame rate', () => {
    expect(streamMetrics({ ...initial, timestamp: 2000 }, initial).fps).toBe(0)
  })
  it('distinguishes unavailable counters from zero', () => {
    const result = streamMetrics({ streamId: 'video', timestamp: 0 })
    expect(result.dropped).toBeNull()
    expect(result.resolution).toBeNull()
    expect(result.mbps).toBeNull()
  })
})
