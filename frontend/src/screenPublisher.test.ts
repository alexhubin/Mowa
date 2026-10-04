import { expect, it, vi, beforeEach } from 'vitest'
import { RoomEvent, Track, type Room } from 'livekit-client'
import { ScreenPublisher } from './screenPublisher'
const policy = vi.hoisted(() => ({ codec: 'vp9' }))
vi.mock('./videoPolicy', () => ({
  RECEIVE_CODECS: 'caps', bitrate: () => 10e6, browserCodecs: () => ({send:['vp9'],receive:['vp9']}),
  chooseCodec: () => policy.codec, parseCodecs: () => ['vp9'],
}))
function fixture(surface = 'browser') {
  const video = { kind: Track.Kind.Video, mediaStreamTrack: {readyState:'live',getSettings:()=>({displaySurface:surface})}, stop:vi.fn(),on:vi.fn(),off:vi.fn() }
  const audio = { kind: Track.Kind.Audio, stop:vi.fn() }
  const handlers = new Map<string,()=>void>()
  const local = { createScreenTracks:vi.fn(async()=>[video,audio]),publishTrack:vi.fn<(...args: unknown[]) => Promise<void>>(async()=>{}),unpublishTrack:vi.fn<(...args: unknown[]) => Promise<void>>(async()=>{}),setAttributes:vi.fn() }
  const room = {localParticipant:local,remoteParticipants:new Map(),on:(event:string,fn:()=>void)=>handlers.set(event,fn),off:vi.fn()} as unknown as Room
  const error=vi.fn();const publisher=new ScreenPublisher(room,error)
  return {video,audio,local,handlers,publisher,error}
}
beforeEach(()=>{policy.codec='vp9'})
it('publishes separate screen audio, preserves it through codec changes, stops both tracks',async()=>{
  const f=fixture();await f.publisher.start(true)
  expect(f.local.publishTrack).toHaveBeenCalledWith(f.audio,expect.objectContaining({source:Track.Source.ScreenShareAudio,forceStereo:true}))
  policy.codec='h264';f.handlers.get(RoomEvent.ParticipantAttributesChanged)!()
  await vi.waitFor(()=>expect(f.local.publishTrack).toHaveBeenCalledTimes(3))
  expect(f.local.publishTrack.mock.calls.filter(([track])=>track===f.audio)).toHaveLength(1)
  await f.publisher.stop()
  expect(f.audio.stop).toHaveBeenCalledOnce();expect(f.video.stop).toHaveBeenCalledOnce()
  expect(f.local.unpublishTrack).toHaveBeenCalledWith(f.audio)
})
it('does not loop browser call audio back through whole-screen capture',async()=>{
  const f=fixture('monitor');await f.publisher.start(true)
  expect(f.audio.stop).toHaveBeenCalledOnce()
  expect(f.local.publishTrack).toHaveBeenCalledTimes(1)
  expect(f.error).toHaveBeenCalledWith(expect.stringContaining('browser tab'))
  await f.publisher.stop()
})
it('stops all captured tracks if audio publication fails',async()=>{
  const f=fixture();f.local.publishTrack.mockResolvedValueOnce(undefined).mockRejectedValueOnce(new Error("Audio publication failed"))
  // An audio failure must also stop and unpublish the already-published video.
  await expect(f.publisher.start(true)).rejects.toThrow('Audio publication failed')
  expect(f.audio.stop).toHaveBeenCalled();expect(f.video.stop).toHaveBeenCalled()
})
