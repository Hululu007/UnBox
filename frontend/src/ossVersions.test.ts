import { describe, expect, it } from 'vitest'
// @ts-expect-error Vitest runs in Node; the app intentionally has no Node type dependency.
import { readFileSync } from 'node:fs'

// 设置页「开源库」的版本是手写的，升级依赖后容易忘记同步（用户已经踩过一次）。
// 用 package.json / go.mod 当权威来源，两边对不上就会挂。
const app = readFileSync('src/App.vue', 'utf8')
const pkg = JSON.parse(readFileSync('package.json', 'utf8'))
const goMod = readFileSync('../go.mod', 'utf8')

const npmDeps: Record<string, string> = {
  vue: 'Vue',
  'hls.js': 'hls.js',
  'mpegts.js': 'mpegts.js',
  dompurify: 'DOMPurify',
}

describe('设置页开源库版本不漂移', () => {
  it('前端依赖版本与 package.json 一致', () => {
    for (const [dep, display] of Object.entries(npmDeps)) {
      const wanted = String(pkg.dependencies[dep]).replace(/^\^/, '')
      if (display === 'Vue') {
        expect(app, `${dep} 显示版本应与 package.json 的 ^${wanted} 一致`).toContain(`>v${wanted}<`)
      } else {
        expect(app, `${dep} 显示版本应与 package.json 的 ^${wanted} 一致`).toContain(`>v${wanted}<`)
      }
    }
  })

  it('Go 依赖版本与 go.mod 一致', () => {
    for (const [mod, display] of [
      ['github.com/wailsapp/wails/v3', 'Wails'],
      ['modernc.org/sqlite', 'modernc.org/sqlite'],
      ['github.com/PuerkitoBio/goquery', 'goquery'],
    ] as const) {
      const line = goMod.split('\n').find((l) => l.includes(`${mod} v`))
      expect(line, `${mod} 应在 go.mod 里`).toBeTruthy()
      const [, version] = line!.trim().split(/\s+/)
      expect(app, `${display} 显示版本应与 go.mod 的 ${version} 一致`).toContain(`>${version}<`)
    }
    // goja 是伪版本，设置页只显示日期；断言日期前缀与 go.mod 一致。
    const gojaLine = goMod.split('\n').find((l) => l.includes('github.com/dop251/goja'))
    const gojaDate = gojaLine!.match(/v0\.0\.0-(\d{8})/)
    expect(gojaDate, 'goja 应为伪版本').toBeTruthy()
    expect(app, 'goja 显示日期应与 go.mod 伪版本一致').toContain(
      `>${gojaDate![1].slice(0, 4)}-${gojaDate![1].slice(4, 6)}-${gojaDate![1].slice(6, 8)}<`,
    )
  })
})