import { useEffect } from 'react'
import { useUIConfig } from '../lib/background'

/**
 * 浏览器标签图标跟随「管理台外观」里配置的 Logo。
 * 没配 Logo 时保留 index.html 里的默认图标。
 */
export function Favicon() {
  const { data } = useUIConfig()
  useEffect(() => {
    const url = (data?.logo ?? '').trim()
    if (!url) return

    let link = document.head.querySelector<HTMLLinkElement>('link[rel="icon"]')
    if (!link) {
      link = document.createElement('link')
      link.rel = 'icon'
      document.head.appendChild(link)
    }
    link.href = url
  }, [data?.logo])
  return null
}
