import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import PlaybackView, { type PlaybackPlan } from './PlaybackView.vue'
import Hls from 'hls.js'
import mpegts from 'mpegts.js'

// 捕获每次 attach() 创建的实例，测试直接调用其注册的错误回调，
// 不依赖 <script setup> 的内部绑定是否泄露到 vm。
const instances = vi.hoisted(() => ({
  hls: [] as any[],
  flv: [] as any[],
}))

vi.mock('hls.js', () => {
  class MockHls {
    static Events = {
      ERROR: 'hlsError',
      MANIFEST_PARSED: 'hlsManifestParsed',
      AUDIO_TRACKS_UPDATED: 'hlsAudioTracksUpdated',
      SUBTITLE_TRACKS_UPDATED: 'hlsSubtitleTracksUpdated',
      LEVEL_SWITCHED: 'hlsLevelSwitched',
      AUDIO_TRACK_SWITCHED: 'hlsAudioTrackSwitched',
      SUBTITLE_TRACK_SWITCH: 'hlsSubtitleTrackSwitch',
    }
    static ErrorTypes = {
      NETWORK_ERROR: 'networkError',
      MEDIA_ERROR: 'mediaError',
      OTHER_ERROR: 'otherError',
    }
    static ErrorDetails = { ATTACH_MEDIA_ERROR: 'attachMediaError' }
    static isSupported = () => true
    handlers: Record<string, Function> = {}
    levels: any[] = []
    currentLevel = -1
    audioTracks: any[] = []
    audioTrack = -1
    subtitleTracks: any[] = []
    subtitleTrack = -1
    on = vi.fn((event: string, cb: Function) => {
      if (event === MockHls.Events.ERROR) (this as any).error = cb
      else this.handlers[event] = cb
    })
    off = vi.fn((event: string) => { delete this.handlers[event] })
    loadSource = vi.fn()
    attachMedia = vi.fn()
    startLoad = vi.fn()
    recoverMediaError = vi.fn()
    destroy = vi.fn()
    constructor() {
      instances.hls.push(this)
    }
  }
  return { default: MockHls, Events: MockHls.Events }
})

vi.mock('mpegts.js', () => {
  class MockFlv {
    static Events = { ERROR: 'flvError' }
    static ErrorTypes = {
      NETWORK_ERROR: 'NetworkError',
      MEDIA_ERROR: 'MediaError',
      OTHER_ERROR: 'OtherError',
    }
    handlers: Record<string, (...args: unknown[]) => void> = {}
    on = vi.fn((event: string, cb: (...args: unknown[]) => void) => {
      this.handlers[event] = cb
    })
    attachMediaElement = vi.fn()
    load = vi.fn()
    unload = vi.fn()
    destroy = vi.fn()
    constructor() {
      instances.flv.push(this)
    }
  }
  return {
    default: {
      getFeatureList: () => ({ mseLivePlayback: true }),
      createPlayer: vi.fn(() => new MockFlv()),
      Events: MockFlv.Events,
      ErrorTypes: MockFlv.ErrorTypes,
    },
  }
})

async function mountView(plan?: Partial<PlaybackPlan>, extraProps: Record<string, unknown> = {}) {
  const wrapper = mount(PlaybackView, {
    props: {
      plan: { ID: 'p1', Backend: 'web', URL: '/x.m3u8', Kind: 'hls', CanFallback: true, ...plan },
      ...extraProps,
    },
  })
  await nextTick()
  await Promise.resolve()
  return wrapper
}

function setVideoTime(wrapper: ReturnType<typeof mount>, seconds: number) {
  ;(wrapper.find('video').element as HTMLVideoElement).currentTime = seconds
}

function hlsError(type: string, fatal = true, details = 'testDetails') {
  return { fatal, type, details }
}

beforeEach(() => {
  instances.hls.length = 0
  instances.flv.length = 0
})

describe('PlaybackView', () => {
  it('attaches HLS to the proxied URL', async () => {
    await mountView()
    expect(instances.hls[0]).toBeTruthy()
    expect(instances.hls[0].loadSource).toHaveBeenCalledWith('/x.m3u8')
  })

  it('ignores non-fatal hls errors', async () => {
    const wrapper = await mountView()
    const hls = instances.hls[0]
    hls.error(Hls.Events.ERROR, hlsError(Hls.ErrorTypes.NETWORK_ERROR, false))
    expect(hls.startLoad).not.toHaveBeenCalled()
    expect(hls.recoverMediaError).not.toHaveBeenCalled()
    expect(wrapper.emitted('fallback')).toBeUndefined()
  })

  it('retries fatal hls network errors in place within the budget', async () => {
    const wrapper = await mountView()
    const hls = instances.hls[0]
    for (let i = 0; i < 3; i++) hls.error(Hls.Events.ERROR, hlsError(Hls.ErrorTypes.NETWORK_ERROR))
    expect(hls.startLoad).toHaveBeenCalledTimes(3)
    expect(wrapper.emitted('fallback')).toBeUndefined()
  })

  it('falls back after the hls network retry budget is exhausted, carrying position', async () => {
    const wrapper = await mountView()
    const hls = instances.hls[0]
    setVideoTime(wrapper, 321)
    for (let i = 0; i < 4; i++) hls.error(Hls.Events.ERROR, hlsError(Hls.ErrorTypes.NETWORK_ERROR))
    expect(hls.startLoad).toHaveBeenCalledTimes(3)
    expect(wrapper.emitted('fallback')).toEqual([['p1', 321]])
  })

  it('recovers from the first fatal hls media error in place', async () => {
    const wrapper = await mountView()
    const hls = instances.hls[0]
    hls.error(Hls.Events.ERROR, hlsError(Hls.ErrorTypes.MEDIA_ERROR))
    expect(hls.recoverMediaError).toHaveBeenCalledTimes(1)
    expect(hls.startLoad).not.toHaveBeenCalled()
    expect(wrapper.emitted('fallback')).toBeUndefined()
  })

  it('falls back after the hls media recovery budget is exhausted', async () => {
    const wrapper = await mountView()
    const hls = instances.hls[0]
    setVideoTime(wrapper, 88)
    // 挂载失败在 1.7.1 里的 type 是 MEDIA_ERROR，细节记在 details。
    const attachFail = () =>
      hls.error(Hls.Events.ERROR, hlsError(Hls.ErrorTypes.MEDIA_ERROR, true, Hls.ErrorDetails.ATTACH_MEDIA_ERROR))
    for (let i = 0; i < 3; i++) attachFail()
    expect(hls.recoverMediaError).toHaveBeenCalledTimes(2)
    expect(wrapper.emitted('fallback')).toEqual([['p1', 88]])
  })

  it('falls back immediately on other fatal hls errors', async () => {
    const wrapper = await mountView()
    const hls = instances.hls[0]
    hls.error(Hls.Events.ERROR, hlsError(Hls.ErrorTypes.OTHER_ERROR))
    expect(hls.startLoad).not.toHaveBeenCalled()
    expect(hls.recoverMediaError).not.toHaveBeenCalled()
    expect(wrapper.emitted('fallback')).toHaveLength(1)
  })

  it('emits fallback at most once', async () => {
    const wrapper = await mountView()
    const hls = instances.hls[0]
    for (let i = 0; i < 5; i++) hls.error(Hls.Events.ERROR, hlsError(Hls.ErrorTypes.OTHER_ERROR))
    expect(wrapper.emitted('fallback')).toHaveLength(1)
  })

  it('does not emit fallback when the plan cannot fall back', async () => {
    const wrapper = await mountView({ CanFallback: false })
    const hls = instances.hls[0]
    for (let i = 0; i < 5; i++) hls.error(Hls.Events.ERROR, hlsError(Hls.ErrorTypes.OTHER_ERROR))
    expect(wrapper.emitted('fallback')).toBeUndefined()
  })

  it('falls back immediately on mpegts media errors, carrying position', async () => {
    const wrapper = await mountView({ Kind: 'flv', URL: '/x.flv' })
    const flv = instances.flv[0]
    setVideoTime(wrapper, 55)
    flv.handlers[mpegts.Events.ERROR](mpegts.ErrorTypes.MEDIA_ERROR, 'MediaMSEError', {})
    expect(flv.unload).not.toHaveBeenCalled()
    expect(wrapper.emitted('fallback')).toEqual([['p1', 55]])
  })

  it('retries mpegts network errors in place, then falls back once the budget is spent', async () => {
    const wrapper = await mountView({ Kind: 'flv', URL: '/x.flv' })
    const flv = instances.flv[0]
    setVideoTime(wrapper, 77)
    const retry = () =>
      flv.handlers[mpegts.Events.ERROR](mpegts.ErrorTypes.NETWORK_ERROR, 'NetworkException', {})

    for (let i = 0; i < 3; i++) retry()
    expect(flv.unload).toHaveBeenCalledTimes(3)
    expect(flv.load).toHaveBeenCalledTimes(4) // 初始加载 + 3 次重连
    expect(wrapper.emitted('fallback')).toBeUndefined()

    retry()
    expect(flv.unload).toHaveBeenCalledTimes(3) // 预算耗尽，不再重连管线
    expect(flv.load).toHaveBeenCalledTimes(4)
    expect(wrapper.emitted('fallback')).toEqual([['p1', 77]])
  })

  it('renders mpv mode without creating a video source', () => {
    const wrapper = mount(PlaybackView, {
      props: { plan: { ID: 'p2', Backend: 'mpv', URL: '', Kind: 'hevc', CanFallback: false } },
    })
    expect(wrapper.find('video').exists()).toBe(false)
    expect(wrapper.text()).toContain('mpv')
  })

  it('shows the default empty text with no plan', () => {
    const wrapper = mount(PlaybackView, { props: { plan: null } })
    expect(wrapper.find('video').exists()).toBe(false)
    expect(wrapper.text()).toContain('选择频道或剧集开始播放')
  })

  it('shows the empty text the caller provides', () => {
    const wrapper = mount(PlaybackView, {
      props: { plan: null, emptyText: '点右侧视频开始播放' },
    })
    expect(wrapper.find('video').exists()).toBe(false)
    expect(wrapper.text()).toContain('点右侧视频开始播放')
    expect(wrapper.text()).not.toContain('选择频道或剧集开始播放')
  })

  it('hls 路径：manifest 后显示设置按钮，点开菜单含清晰度项', async () => {
    const wrapper = await mountView()
    const hls = instances.hls[instances.hls.length - 1]
    hls.levels = [{ height: 720, bitrate: 2_800_000 }]
    hls.handlers[Hls.Events.MANIFEST_PARSED]({})
    await nextTick()
    expect(wrapper.find('.track-toggle').exists()).toBe(true)
    await wrapper.find('.track-toggle').trigger('click')
    expect(wrapper.find('.track-menu').exists()).toBe(true)
    expect(wrapper.text()).toContain('720p')
  })

  it('flv 路径不渲染设置按钮', async () => {
    const wrapper = await mountView({ Kind: 'flv' })
    await nextTick()
    expect(wrapper.find('.track-toggle').exists()).toBe(false)
  })

  it('mpv 后端不渲染设置按钮', async () => {
    const wrapper = await mountView({ Backend: 'mpv' })
    await nextTick()
    expect(wrapper.find('.track-toggle').exists()).toBe(false)
  })

  it('点击菜单外关闭轨道菜单', async () => {
    const wrapper = await mountView()
    const hls = instances.hls[instances.hls.length - 1]
    hls.handlers[Hls.Events.MANIFEST_PARSED]({})
    await nextTick()
    await wrapper.find('.track-toggle').trigger('click')
    expect(wrapper.find('.track-menu').exists()).toBe(true)
    document.body.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    expect(wrapper.find('.track-menu').exists()).toBe(false)
  })

  it('快速切换播放计划时只挂载最新 HLS', async () => {
    const wrapper = mount(PlaybackView, {
      props: { plan: { ID: 'old', Backend: 'web', URL: '/old.m3u8', Kind: 'hls', CanFallback: true } },
    })
    await wrapper.setProps({ plan: { ID: 'new', Backend: 'web', URL: '/new.m3u8', Kind: 'hls', CanFallback: true } })
    await nextTick()
    await Promise.resolve()
    expect(instances.hls).toHaveLength(2)
    expect(instances.hls[0].destroy).toHaveBeenCalled()
    expect(instances.hls[1].loadSource).toHaveBeenCalledWith('/new.m3u8')
  })

  it('把原生 video 事件统一上报为标准播放信号', async () => {
    const wrapper = await mountView()
    const video = wrapper.find('video')
    await video.trigger('playing')
    await video.trigger('waiting')
    await video.trigger('stalled')
    await video.trigger('canplay')
    await video.trigger('ended')
    expect(wrapper.emitted('playback')).toEqual([
      ['playing'],
      ['buffering'],
      ['buffering'],
      ['ready'],
      ['ended'],
    ])
  })

  it('suppressFallback 为真时只上报错误，不发旧 fallback', async () => {
    const wrapper = await mountView({ Kind: 'flv', URL: '/x.flv' }, { suppressFallback: true })
    const flv = instances.flv[0]
    flv.handlers[mpegts.Events.ERROR](mpegts.ErrorTypes.MEDIA_ERROR, 'MediaMSEError', {})
    expect(wrapper.emitted('fallback')).toBeUndefined()
    expect(wrapper.emitted('playback')).toEqual([['error', 'MediaMSEError']])
  })

  it('suppressFallback 关闭时保留原有 fallback 行为', async () => {
    const wrapper = await mountView({ Kind: 'flv', URL: '/x.flv' })
    const flv = instances.flv[0]
    setVideoTime(wrapper, 42)
    flv.handlers[mpegts.Events.ERROR](mpegts.ErrorTypes.MEDIA_ERROR, 'MediaMSEError', {})
    expect(wrapper.emitted('fallback')).toEqual([['p1', 42]])
  })

  it('每次播放计划只上报一次错误信号', async () => {
    const wrapper = await mountView({ Kind: 'flv', URL: '/x.flv' }, { suppressFallback: true })
    const flv = instances.flv[0]
    const fire = () => flv.handlers[mpegts.Events.ERROR](mpegts.ErrorTypes.MEDIA_ERROR, 'MediaMSEError', {})
    fire()
    fire()
    fire()
    expect(wrapper.emitted('playback')).toEqual([['error', 'MediaMSEError']])
  })

  it('原生 video error 事件上报错误信号', async () => {
    const wrapper = await mountView({ Kind: 'mp4', URL: '/x.mp4' }, { suppressFallback: true })
    await wrapper.find('video').trigger('error')
    expect(wrapper.emitted('playback')).toEqual([['error', undefined]])
  })

  it('旋转按钮按 90° 步进切换画面角度', async () => {
    const wrapper = await mountView()
    const button = wrapper.find('.rotate-toggle')
    expect(button.exists()).toBe(true)
    expect(button.text()).toContain('0°')
    const video = wrapper.find('video').element as HTMLVideoElement

    await button.trigger('click')
    expect(button.text()).toContain('90°')
    expect(video.style.transform).toContain('rotate(90deg)')

    await button.trigger('click')
    expect(video.style.transform).toContain('rotate(180deg)')

    await button.trigger('click')
    expect(video.style.transform).toContain('rotate(270deg)')

    await button.trigger('click')
    expect(button.text()).toContain('0°')
    expect(video.style.transform).toBe('')
  })

  it('流就绪后自动开始播放，不需要用户再点一次', async () => {
    const wrapper = await mountView()
    const video = wrapper.find('video').element as HTMLVideoElement
    const play = vi.fn(() => Promise.resolve())
    video.play = play as unknown as HTMLVideoElement['play']
    await wrapper.find('video').trigger('canplay')
    expect(play).toHaveBeenCalled()
  })

  it('原生 video 全屏时改由整个播放容器全屏，保住右上角控件', async () => {
    const wrapper = await mountView()
    const video = wrapper.find('video').element as HTMLVideoElement
    const box = video.parentElement as HTMLElement
    const request = vi.fn(() => Promise.resolve())
    box.requestFullscreen = request as unknown as HTMLElement['requestFullscreen']
    Object.defineProperty(document, 'fullscreenElement', { configurable: true, get: () => video })
    document.dispatchEvent(new Event('fullscreenchange'))
    expect(request).toHaveBeenCalled()
    Object.defineProperty(document, 'fullscreenElement', { configurable: true, get: () => null })
  })

  it('自绘播放控件默认隐藏，鼠标靠近才显示，播放中静置后自动隐藏', async () => {
    vi.useFakeTimers()
    try {
      const wrapper = await mountView()
      const root = wrapper.find('.playback-view')
      expect(root.classes()).not.toContain('controls-visible')

      await root.trigger('mousemove')
      expect(root.classes()).toContain('controls-visible')

      // 暂停状态下不自动隐藏，避免用户找不到控件
      vi.advanceTimersByTime(5000)
      await nextTick()
      expect(root.classes()).toContain('controls-visible')

      await wrapper.find('video').trigger('playing')
      vi.advanceTimersByTime(3000)
      await nextTick()
      expect(root.classes()).not.toContain('controls-visible')
    } finally {
      vi.useRealTimers()
    }
  })

  it('自绘控件提供播放/暂停与进度定位', async () => {
    const wrapper = await mountView()
    const video = wrapper.find('video').element as HTMLVideoElement
    const pause = vi.fn()
    video.pause = pause as unknown as HTMLVideoElement['pause']
    Object.defineProperty(video, 'paused', { configurable: true, get: () => false })

    await wrapper.findAll('.ctrl-btn')[0].trigger('click')
    expect(pause).toHaveBeenCalled()

    Object.defineProperty(video, 'duration', { configurable: true, get: () => 120 })
    await wrapper.find('video').trigger('timeupdate')
    await wrapper.find('.ctrl-seek').setValue('42')
    expect(video.currentTime).toBe(42)
  })

  it('全屏按钮请求整个播放容器全屏', async () => {
    const wrapper = await mountView()
    const box = wrapper.find('video').element.parentElement as HTMLElement
    const request = vi.fn(() => Promise.resolve())
    box.requestFullscreen = request as unknown as HTMLElement['requestFullscreen']

    const buttons = wrapper.findAll('.ctrl-btn')
    await buttons[buttons.length - 1].trigger('click')
    expect(request).toHaveBeenCalled()
  })

  it('loading 为真时显示加载层（仅转圈，加载文案由软件底部 toast 承担）', async () => {
    const wrapper = mount(PlaybackView, { props: { plan: null, loading: true } })
    expect(wrapper.find('.player-loading').exists()).toBe(true)
    expect(wrapper.find('.player-loading-spinner').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('正在加载剧集')
    await wrapper.setProps({ loading: false })
    expect(wrapper.find('.player-loading').exists()).toBe(false)
  })

  it('片头/片尾按钮回显绝对标记点，右键重置', async () => {
    const wrapper = mount(PlaybackView, {
      props: { plan: { ID: 'p1', Backend: 'web', URL: '/x.m3u8', Kind: 'hls', CanFallback: true } },
    })
    await nextTick()
    const video = wrapper.find('video').element as HTMLVideoElement
    Object.defineProperty(video, 'duration', { configurable: true, get: () => 1260 })
    await wrapper.find('video').trigger('timeupdate')

    // 未标记：片头 00:00，片尾回显总时长
    const intro = wrapper.find('button[aria-label="片头标记"]')
    const outro = wrapper.find('button[aria-label="片尾标记"]')
    expect(intro.text()).toContain('00:00')
    expect(outro.text()).toContain('21:00')

    setVideoTime(wrapper, 90)
    await intro.trigger('click')
    expect(wrapper.emitted('markIntro')).toEqual([[90]])
    setVideoTime(wrapper, 1200)
    await outro.trigger('click')
    expect(wrapper.emitted('markOutro')).toEqual([[1200]])

    await intro.trigger('contextmenu')
    await outro.trigger('contextmenu')
    expect(wrapper.emitted('resetIntro')).toHaveLength(1)
    expect(wrapper.emitted('resetOutro')).toHaveLength(1)
  })

  it('mpv 后端不渲染自绘播放控件', async () => {
    const wrapper = await mountView({ Backend: 'mpv' })
    expect(wrapper.find('.player-controls').exists()).toBe(false)
  })

  it('倍速菜单可切换播放速度', async () => {
    const wrapper = await mountView()
    const video = wrapper.find('video').element as HTMLVideoElement
    await wrapper.find('.rate-btn').trigger('click')
    const option = wrapper.findAll('.rate-menu li').find((item) => item.text() === '1.5×')
    expect(option).toBeTruthy()
    await option!.trigger('click')
    expect(video.playbackRate).toBe(1.5)
    expect(wrapper.find('.rate-menu').exists()).toBe(false)
  })

  it('支持画中画时提供按钮并可进入画中画', async () => {
    Object.defineProperty(document, 'pictureInPictureEnabled', { configurable: true, value: true })
    const request = vi.fn(() => Promise.resolve({} as PictureInPictureWindow))
    HTMLVideoElement.prototype.requestPictureInPicture = request as unknown as HTMLVideoElement['requestPictureInPicture']
    try {
      const wrapper = await mountView()
      const button = wrapper.find('button[aria-label="画中画"]')
      expect(button.exists()).toBe(true)
      await button.trigger('click')
      expect(request).toHaveBeenCalled()
    } finally {
      Object.defineProperty(document, 'pictureInPictureEnabled', { configurable: true, value: undefined })
      delete (HTMLVideoElement.prototype as unknown as Record<string, unknown>).requestPictureInPicture
    }
  })

  it('单击画面切换播放/暂停，双击则只切全屏', async () => {
    vi.useFakeTimers()
    try {
      const wrapper = await mountView()
      const video = wrapper.find('video').element as HTMLVideoElement
      const box = video.parentElement as HTMLElement
      const pause = vi.fn()
      video.pause = pause as unknown as HTMLVideoElement['pause']
      Object.defineProperty(video, 'paused', { configurable: true, get: () => false })

      await wrapper.find('video').trigger('click')
      vi.advanceTimersByTime(250)
      expect(pause).toHaveBeenCalledTimes(1)

      // 双击时取消单击的播放/暂停动作，只做全屏
      pause.mockClear()
      const fullscreen = vi.fn(() => Promise.resolve())
      box.requestFullscreen = fullscreen as unknown as HTMLElement['requestFullscreen']
      await wrapper.find('video').trigger('click')
      await wrapper.find('video').trigger('dblclick')
      vi.advanceTimersByTime(250)
      expect(pause).not.toHaveBeenCalled()
      expect(fullscreen).toHaveBeenCalled()
    } finally {
      vi.useRealTimers()
    }
  })

})

describe('刷新与音量控件（2026-10-22）', () => {
  it('刷新按钮重建媒体并在元数据就绪后回到刷新前位置', async () => {
    const wrapper = await mountView()
    const before = instances.hls.length
    setVideoTime(wrapper, 123.4)
    await wrapper.find('button[aria-label="刷新当前视频"]').trigger('click')
    await nextTick()
    await Promise.resolve()
    // 重建：旧 hls 被销毁、新建一个（旧实例是数组里的前一个）
    expect(instances.hls.length).toBe(before + 1)
    expect(instances.hls[before - 1].destroy).toHaveBeenCalled()
    // 元数据就绪后 seek 回刷新前位置
    const video = wrapper.find('video').element as HTMLVideoElement
    video.dispatchEvent(new Event('loadedmetadata'))
    expect(video.currentTime).toBe(123.4)
  })

  it('音量改为悬停弹层：横向条不再内联，静音点击切换有效', async () => {
    const wrapper = await mountView()
    // 内联横向条已移除，纵向条搬进 volume-menu
    expect(wrapper.find('.player-bar > input[type="range"]').exists()).toBe(false)
    const menu = wrapper.find('.volume-menu')
    expect(menu.exists()).toBe(true)
    expect(menu.find('input.ctrl-volume[type="range"]').exists()).toBe(true)
    // 点击静音按钮切换静音
    const video = wrapper.find('video').element as HTMLVideoElement
    video.muted = false
    await wrapper.find('.volume-control .ctrl-btn').trigger('click')
    expect(video.muted).toBe(true)
  })

  it('寻迹条悬停出现时间气泡，且百分比按鼠标坐标而非播放进度', async () => {
    const wrapper = await mountView()
    const video = wrapper.find('video').element as HTMLVideoElement
    Object.defineProperty(video, 'duration', { configurable: true, get: () => 120 })
    const seek = wrapper.find('input.ctrl-seek')
    expect(seek.exists()).toBe(true)
    // jsdom 无布局：桩 getBoundingClientRect 让坐标换算可用。
    const rect = { left: 0, width: 100, top: 0, right: 100, bottom: 0, height: 10, x: 0, y: 0, toJSON: () => ({}) }
    Object.defineProperty(seek.element, 'getBoundingClientRect', { configurable: true, value: () => rect })
    setVideoTime(wrapper, 3) // 正在播到 3 秒
    await wrapper.find('video').trigger('timeupdate') // 让 duration ref 吃到 120
    await seek.trigger('mousemove', { clientX: 50 })
    // 悬停在中点 → 气泡显示 01:00（60s），而不是播放进度 00:03
    expect(wrapper.find('.seek-bubble').text()).toBe('01:00')
  })
})
