/**
 * 从背景图里取主色，派生出管理台整套配色令牌。
 *
 * 纯前端实现：把图片缩到 40×40 画到 canvas 上做色相直方图，取权重最高的色相簇当基色，
 * 再按亮/暗色分别派生 accent（强调色）与基色（背景/表面/分割线/文字）。
 *
 * 基色只做「轻微染色」，沿用原来的明度阶梯，保留纸感而不至于变成彩色主题。
 *
 * 取不到时返回 null（图片跨域没放行 / 灰阶图 / 加载失败），调用方保持默认配色。
 */

export type ThemePalette = {
  accent: string
  accentHover: string
  accentInk: string
  accentSoft: string
  accentText: string
  background: string
  surface: string
  subtle: string
  line: string
  scroll: string
  ink: string
  inkSoft: string
  inkFaint: string
  focus: string
  navSolid: string
}

/** 派生出来的 CSS 变量名；clearPalette() 需要按同一份清单清理。 */
export const THEME_VARS = [
  '--accent',
  '--accent-hover',
  '--accent-ink',
  '--accent-soft',
  '--accent-text',
  '--background',
  '--surface',
  '--subtle',
  '--line',
  '--scroll',
  '--ink',
  '--ink-soft',
  '--ink-faint',
  '--focus',
  '--nav-solid',
] as const

// 壁纸下基色是半透明的, 数值与 index.css 的 html.wallpaper 保持一致,
// 否则内联变量会盖掉那条规则、壁纸就透不出来了。
const ALPHA = { background: 0.55, surface: 0.72, subtle: 0.6 }

const SAMPLE = 40 // 采样边长，40×40 足够判断主色又不吃 CPU
const HUE_BUCKETS = 24
// 可用像素占比低于这个值就认为图里没有主色（纯黑白或大面积灰）
const MIN_SHARE = 0.02

type Hsl = { h: number; s: number; l: number }

function clamp(v: number, lo: number, hi: number): number {
  return Math.min(hi, Math.max(lo, v))
}

function rgbToHsl(r: number, g: number, b: number): Hsl {
  const rn = r / 255
  const gn = g / 255
  const bn = b / 255
  const max = Math.max(rn, gn, bn)
  const min = Math.min(rn, gn, bn)
  const l = (max + min) / 2
  const d = max - min
  if (d === 0) return { h: 0, s: 0, l }
  const s = l > 0.5 ? d / (2 - max - min) : d / (max + min)
  let h: number
  if (max === rn) h = ((gn - bn) / d + (gn < bn ? 6 : 0)) / 6
  else if (max === gn) h = ((bn - rn) / d + 2) / 6
  else h = ((rn - gn) / d + 4) / 6
  return { h: h * 360, s, l }
}

function hsl(h: number, s: number, l: number, a?: number): string {
  const body = `${h.toFixed(0)} ${(clamp(s, 0, 1) * 100).toFixed(0)}% ${(clamp(l, 0, 1) * 100).toFixed(0)}%`
  return a === undefined ? `hsl(${body})` : `hsl(${body} / ${a})`
}

function loadImage(url: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image()
    // 必须 anonymous：否则 canvas 被跨域污染，getImageData 会直接抛异常
    img.crossOrigin = 'anonymous'
    img.referrerPolicy = 'no-referrer'
    img.decoding = 'async'
    img.onload = () => resolve(img)
    img.onerror = () => reject(new Error('背景图加载失败'))
    img.src = url
  })
}

/** 统计色相直方图，返回权重最高那一簇的平均色相/饱和度/亮度。 */
function dominantOf(img: HTMLImageElement): Hsl | null {
  const canvas = document.createElement('canvas')
  canvas.width = SAMPLE
  canvas.height = SAMPLE
  const ctx = canvas.getContext('2d', { willReadFrequently: true })
  if (!ctx) return null
  ctx.drawImage(img, 0, 0, SAMPLE, SAMPLE)

  let data: Uint8ClampedArray
  try {
    data = ctx.getImageData(0, 0, SAMPLE, SAMPLE).data
  } catch {
    return null // 跨域污染
  }

  type Bucket = { w: number; x: number; y: number; s: number; l: number }
  const buckets: Bucket[] = Array.from({ length: HUE_BUCKETS }, () => ({
    w: 0,
    x: 0,
    y: 0,
    s: 0,
    l: 0,
  }))
  let usable = 0

  for (let i = 0; i < data.length; i += 4) {
    if (data[i + 3] < 128) continue
    const { h, s, l } = rgbToHsl(data[i], data[i + 1], data[i + 2])
    // 太亮/太暗/太灰的像素对「主色」没有指示意义
    if (l > 0.92 || l < 0.08 || s < 0.12) continue
    usable++
    // 饱和度高的像素更能代表主色，中等亮度再给一点加成
    const w = s * (1 - Math.abs(l - 0.55) * 1.1)
    if (w <= 0) continue
    const bucket = buckets[Math.floor((h / 360) * HUE_BUCKETS) % HUE_BUCKETS]
    bucket.w += w
    bucket.s += s * w
    bucket.l += l * w
    // 用单位向量累加求环形平均，避免 0°/359° 被拆成两簇
    const rad = (h * Math.PI) / 180
    bucket.x += Math.cos(rad) * w
    bucket.y += Math.sin(rad) * w
  }

  if (usable / (SAMPLE * SAMPLE) < MIN_SHARE) return null

  let best = buckets[0]
  for (const bucket of buckets) if (bucket.w > best.w) best = bucket
  if (best.w <= 0) return null

  let hue = (Math.atan2(best.y, best.x) * 180) / Math.PI
  if (hue < 0) hue += 360
  return { h: hue, s: best.s / best.w, l: best.l / best.w }
}

/**
 * 由基色派生整套配色。
 *
 * accent：亮色下必须偏深才在米白背景上可读，暗色下反过来；饱和度做上下限夹取，
 * 避免接近灰的图取出来的颜色看不出来。
 *
 * 基色：只位移色相、加一点极低饱和度，明度阶梯沿用原来的（背景→表面→分割线→文字），
 * 所以观感还是同一套纸感界面，只是被背景图「染」了一层色调。
 */
function paletteOf(base: Hsl, dark: boolean): ThemePalette {
  const h = base.h
  const s = clamp(base.s, 0.25, 0.62)
  // 中性色用的饱和度：0.08 ~ 0.12，足以看出色调但不抢戏
  const n = 0.05 + s * 0.12
  if (dark) {
    return {
      accent: hsl(h, s * 0.35, 0.9),
      accentHover: hsl(h, s * 0.5, 0.78),
      accentInk: hsl(h, s * 0.3, 0.12),
      accentSoft: hsl(h, s * 0.45, 0.19),
      accentText: hsl(h, s * 0.7, 0.74),
      background: hsl(h, n, 0.1, ALPHA.background),
      surface: hsl(h, n, 0.15, ALPHA.surface),
      subtle: hsl(h, n, 0.21, ALPHA.subtle),
      line: hsl(h, n * 0.9, 0.27),
      scroll: hsl(h, n, 0.31),
      ink: hsl(h, n * 1.2, 0.95),
      inkSoft: hsl(h, n * 0.8, 0.79),
      inkFaint: hsl(h, n * 0.6, 0.56),
      focus: hsl(h, n * 0.5, 0.42),
      navSolid: hsl(h, n, 0.14),
    }
  }
  return {
    accent: hsl(h, s * 0.4, 0.17),
    accentHover: hsl(h, s * 0.6, 0.29),
    accentInk: hsl(h, s * 0.2, 0.97),
    accentSoft: hsl(h, s * 0.45, 0.91),
    accentText: hsl(h, s * 0.65, 0.36),
    background: hsl(h, n * 1.1, 0.97, ALPHA.background),
    surface: hsl(h, n * 0.7, 0.995, ALPHA.surface),
    subtle: hsl(h, n, 0.94, ALPHA.subtle),
    line: hsl(h, n * 0.9, 0.9),
    scroll: hsl(h, n, 0.86),
    ink: hsl(h, n * 1.2, 0.16),
    inkSoft: hsl(h, n * 0.8, 0.35),
    inkFaint: hsl(h, n * 0.6, 0.64),
    focus: hsl(h, n * 0.5, 0.64),
    navSolid: hsl(h, n * 0.8, 0.995),
  }
}

const cache = new Map<string, ThemePalette | null>()

/** 取背景图主色并派生完整配色；结果按 图 URL + 亮暗 缓存。 */
export async function deriveTheme(url: string, dark: boolean): Promise<ThemePalette | null> {
  const key = `${dark ? 'dark' : 'light'} ${url}`
  const hit = cache.get(key)
  if (hit !== undefined) return hit
  let palette: ThemePalette | null = null
  try {
    const img = await loadImage(url)
    const base = dominantOf(img)
    if (base) palette = paletteOf(base, dark)
  } catch {
    palette = null
  }
  cache.set(key, palette)
  return palette
}

export function applyPalette(root: HTMLElement, palette: ThemePalette) {
  root.style.setProperty('--accent', palette.accent)
  root.style.setProperty('--accent-hover', palette.accentHover)
  root.style.setProperty('--accent-ink', palette.accentInk)
  root.style.setProperty('--accent-soft', palette.accentSoft)
  root.style.setProperty('--accent-text', palette.accentText)
  root.style.setProperty('--background', palette.background)
  root.style.setProperty('--surface', palette.surface)
  root.style.setProperty('--subtle', palette.subtle)
  root.style.setProperty('--line', palette.line)
  root.style.setProperty('--scroll', palette.scroll)
  root.style.setProperty('--ink', palette.ink)
  root.style.setProperty('--ink-soft', palette.inkSoft)
  root.style.setProperty('--ink-faint', palette.inkFaint)
  root.style.setProperty('--focus', palette.focus)
  root.style.setProperty('--nav-solid', palette.navSolid)
}

export function clearPalette(root: HTMLElement) {
  for (const name of THEME_VARS) root.style.removeProperty(name)
}
