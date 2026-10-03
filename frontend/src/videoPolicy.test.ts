import { describe, expect, it } from 'vitest'
import { bitrate, chooseCodec, codecOrder, fromCapabilities, parseCodecs } from './videoPolicy'

describe('common screen codec', () => {
  it('follows priority, membership changes and sender capabilities', () => {
    expect(chooseCodec(codecOrder, [codecOrder])).toBe('av1')
    expect(chooseCodec(codecOrder, [codecOrder, ['vp9', 'h264']])).toBe('vp9')
    expect(chooseCodec(codecOrder, [['h264'], ['vp9', 'h264']])).toBe('h264')
    expect(chooseCodec(['vp9', 'h264'], [codecOrder])).toBe('vp9')
    expect(chooseCodec(codecOrder, [])).toBe('av1')
    expect(chooseCodec(['vp9'], [['h264']])).toBeUndefined()
    expect(chooseCodec(codecOrder, [[]])).toBeUndefined()
  })
  it('handles legacy clients without inventing support for explicit empty lists', () => {
    expect(parseCodecs(undefined)).toEqual(['h264'])
    expect(parseCodecs('')).toEqual([])
    expect(parseCodecs('vp9,av1,h265,vp8')).toEqual(['av1', 'vp9'])
  })
  it('accepts compatible RTP profiles only', () => {
    expect(fromCapabilities([
      { mimeType: 'video/VP9', sdpFmtpLine: 'profile-id=2' },
      { mimeType: 'video/AV1', sdpFmtpLine: 'profile=1' },
      { mimeType: 'video/H264', sdpFmtpLine: 'profile-level-id=640c1f;packetization-mode=1' },
    ])).toEqual([])
    expect(fromCapabilities([
      { mimeType: 'video/VP9' },
      { mimeType: 'video/AV1', sdpFmtpLine: 'profile=0' },
      { mimeType: 'video/H264', sdpFmtpLine: 'profile-level-id=42e01f;packetization-mode=1' },
    ])).toEqual(codecOrder)
  })
  it('uses the exact budgets at both resolutions', () => {
    expect(codecOrder.map(c => bitrate(c, true))).toEqual([8e6, 10e6, 14e6])
    expect(codecOrder.map(c => bitrate(c, false))).toEqual([4e6, 5e6, 7e6])
  })
})
