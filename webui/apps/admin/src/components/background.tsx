import { useEffect } from 'react'
import { useUIConfig } from '../lib/background'

/**
 * 全屏背景层：固定在内容之后，按配置的不透明度显示背景图。
 *
 * 有壁纸时给 <html> 挂 `wallpaper` 类，index.css 会据此把 background/surface/subtle
 * 换成半透明版本 —— 否则页面里的纯色容器会把壁纸完全挡住。
 */
export function BackgroundLayer() {
  const { data } = useUIConfig()
  const url = (data?.background ?? '').trim()
  const opacity = data?.background_opacity || 30

  useEffect(() => {
    const root = document.documentElement
    root.classList.toggle('wallpaper', url !== '')
    return () => root.classList.remove('wallpaper')
  }, [url])

  if (!url) return null
  return (
    <div
      aria-hidden
      className="pointer-events-none fixed inset-0 -z-10 bg-cover bg-center"
      style={{ backgroundImage: `url("${url}")`, opacity: Math.min(100, Math.max(0, opacity)) / 100 }}
    />
  )
}
