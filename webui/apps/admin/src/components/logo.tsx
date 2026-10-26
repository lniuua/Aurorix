import { cn } from '@aurorix/webui'
import { useUIConfig } from '../lib/background'

/** 登录页与侧边栏共用的品牌 Logo：配置了图片就用图，否则回退默认字标。 */
export function Logo({ size = 'md' }: { size?: 'md' | 'lg' }) {
  const { data } = useUIConfig()
  const url = (data?.logo ?? '').trim()
  const px = size === 'lg' ? 'size-11 text-[20px]' : 'size-10 text-[19px]'
  if (url) {
    return (
      <img
        src={url}
        alt="logo"
        referrerPolicy="no-referrer"
        className={cn('shrink-0 rounded-[1.25rem] object-cover', px)}
      />
    )
  }
  return (
    <span
      className={cn(
        'grid shrink-0 place-items-center rounded-[1.25rem] border border-accent/25 bg-accent font-serif font-semibold text-accent-ink',
        px,
      )}
    >
      A
    </span>
  )
}
