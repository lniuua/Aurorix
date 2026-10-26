import { useUIConfig } from '../lib/background'

/** 全屏背景层：固定在内容之后，按配置的不透明度显示背景图。 */
export function BackgroundLayer() {
  const { data } = useUIConfig()
  const url = data?.background ?? ''
  const opacity = data?.background_opacity || 30
  if (!url) return null
  return (
    <div
      aria-hidden
      className="pointer-events-none fixed inset-0 -z-10 bg-cover bg-center"
      style={{ backgroundImage: `url("${url}")`, opacity: Math.min(100, Math.max(0, opacity)) / 100 }}
    />
  )
}
