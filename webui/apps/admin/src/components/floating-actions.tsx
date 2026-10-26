import type { ReactNode } from 'react'
import { cn } from '@aurorix/webui'

/**
 * 右下角悬浮操作条。
 *
 * 长表单页面（设置、插件权限等）的保存按钮原本在最顶部，滚到底部改完还得翻回去；
 * 这里把主操作钉在视口右下角，随时可点。放在 z-20：低于移动端抽屉(40)与弹窗(40)。
 */
export function FloatingActions({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div
      className={cn(
        'pointer-events-none fixed inset-x-0 bottom-0 z-20 flex justify-end px-4 md:px-8',
        'pb-[max(1rem,env(safe-area-inset-bottom))]',
        className,
      )}
    >
      <div className="pointer-events-auto flex items-center gap-1.5 rounded-full border border-line bg-[var(--nav-solid)] py-1.5 pl-3.5 pr-1.5 shadow-[0_12px_32px_-12px_rgb(0_0_0/0.45)]">
        {children}
      </div>
    </div>
  )
}
