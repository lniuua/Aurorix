import { useEffect } from 'react'
import { useUIConfig } from '../lib/background'
import { applyPalette, clearPalette, deriveTheme } from '../lib/theme-color'

/**
 * 按背景图自动配色。
 *
 * 开启壁纸且 auto_theme 未关闭时，从图里提取主色派生整套令牌（强调色 + 基色），
 * 写到 <html> 的内联样式上。内联变量会盖过 `.dark` 与 `html.wallpaper` 里的定义，
 * 所以亮暗切换后必须重算一次，且基色要自带壁纸的半透明值。
 */
export function AutoTheme() {
  const { data } = useUIConfig()
  const url = (data?.background ?? '').trim()
  const enabled = data?.auto_theme !== false

  useEffect(() => {
    const root = document.documentElement
    let cancelled = false
    let lastDark: boolean | null = null

    const apply = async () => {
      if (!url || !enabled) {
        clearPalette(root)
        return
      }
      const dark = root.classList.contains('dark')
      lastDark = dark
      const palette = await deriveTheme(url, dark)
      if (cancelled) return
      if (!palette) {
        // 取不到（跨域/灰阶图）就整体退回默认配色，而不是留下一半变量
        clearPalette(root)
        return
      }
      applyPalette(root, palette)
    }

    void apply()

    const observer = new MutationObserver(() => {
      const dark = root.classList.contains('dark')
      if (dark === lastDark) return
      void apply()
    })
    observer.observe(root, { attributes: true, attributeFilter: ['class'] })

    return () => {
      cancelled = true
      observer.disconnect()
      clearPalette(root)
    }
  }, [url, enabled])

  return null
}
