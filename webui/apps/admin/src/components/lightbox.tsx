import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { createPortal } from 'react-dom'
import { X, ZoomIn, ZoomOut } from 'lucide-react'
import { cn } from '@aurorix/webui'

/** 图片加载失败时的灰色占位图。 */
const BROKEN_IMAGE_PATH =
  'M1308.444444 1024H333.653333L711.111111 369.777778l142.222222-142.222222 28.444445-170.666667 85.333333-56.888889h341.333333a56.888889 56.888889 0 0 1 56.888889 56.888889v910.222222a56.888889 56.888889 0 0 1-56.888889 56.888889z m-245.76-654.222222l-189.297777 438.186666-133.262223-262.200888L540.444444 938.666667h768l-245.76-568.888889zM711.111111 227.555556l-71.281778 83.171555L540.444444 426.666667l-224 597.333333H56.888889a56.888889 56.888889 0 0 1-56.888889-56.888889V56.888889a56.888889 56.888889 0 0 1 56.888889-56.888889h711.111111s-28.444444 102.4-28.444444 113.777778c0 11.377778-28.444444 113.777778-28.444445 113.777778zM341.333333 170.666667a142.222222 142.222222 0 1 0 0 284.444444 142.222222 142.222222 0 0 0 0-284.444444z'

export function BrokenImage({ className }: { className?: string }) {
  return (
    <div className={cn('grid shrink-0 place-items-center rounded-xl bg-subtle p-4', className)}>
      <svg viewBox="0 0 1365 1024" className="h-16 w-auto" role="img" aria-label="图片加载失败">
        <path d={BROKEN_IMAGE_PATH} fill="#A6A6A6" />
      </svg>
    </div>
  )
}

const ImagePreviewContext = createContext<(src: string) => void>(() => {})

/**
 * 提供「点图放大」能力：子树里的任何 ZoomImage 点一下都会打开灯箱。
 * 挂在 AppShell 上，各页面不必各自维护灯箱状态。
 */
export function ImagePreviewProvider({ children }: { children: ReactNode }) {
  const [src, setSrc] = useState<string | null>(null)
  return (
    <ImagePreviewContext.Provider value={setSrc}>
      {children}
      {src && <Lightbox src={src} onClose={() => setSrc(null)} />}
    </ImagePreviewContext.Provider>
  )
}

/** 可点击放大的图片；加载失败自动回退占位图。 */
export function ZoomImage({ src, alt, className }: { src?: string; alt?: string; className?: string }) {
  const open = useContext(ImagePreviewContext)
  const [failed, setFailed] = useState(false)
  useEffect(() => setFailed(false), [src])
  if (!src) return null
  if (failed) return <BrokenImage />
  return (
    <img
      src={src}
      alt={alt ?? ''}
      referrerPolicy="no-referrer"
      className={cn('cursor-zoom-in', className)}
      onClick={() => open(src)}
      onError={() => setFailed(true)}
    />
  )
}

/** 全屏看图灯箱：滚轮/按钮/双击缩放，放大后可拖拽平移；点遮罩或按 Esc 关闭。 */
export function Lightbox({ src, onClose }: { src: string; onClose: () => void }) {
  const [failed, setFailed] = useState(false)
  const [view, setView] = useState({ s: 1, x: 0, y: 0 })
  const [dragging, setDragging] = useState(false)
  const overlayRef = useRef<HTMLDivElement>(null)
  const drag = useRef<{ x: number; y: number; vx: number; vy: number } | null>(null)

  useEffect(() => {
    setFailed(false)
    setView({ s: 1, x: 0, y: 0 })
  }, [src])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      window.removeEventListener('keydown', onKey)
      document.body.style.overflow = prev
    }
  }, [onClose])

  // 以光标(默认视口中心)为锚点缩放；缩回 1x 时偏移清零。
  const zoom = useCallback((factor: number, clientX?: number, clientY?: number) => {
    setView((v) => {
      const s = Math.min(8, Math.max(1, v.s * factor))
      if (s === v.s) return v
      if (s === 1) return { s: 1, x: 0, y: 0 }
      const cx = (clientX ?? window.innerWidth / 2) - window.innerWidth / 2
      const cy = (clientY ?? window.innerHeight / 2) - window.innerHeight / 2
      const k = s / v.s
      return { s, x: v.x + (cx - v.x) * (1 - k), y: v.y + (cy - v.y) * (1 - k) }
    })
  }, [])

  // React 的 onWheel 是 passive，preventDefault 不生效，这里挂原生非 passive 监听。
  useEffect(() => {
    const el = overlayRef.current
    if (!el) return
    const handler = (e: WheelEvent) => {
      e.preventDefault()
      zoom(e.deltaY < 0 ? 1.15 : 1 / 1.15, e.clientX, e.clientY)
    }
    el.addEventListener('wheel', handler, { passive: false })
    return () => el.removeEventListener('wheel', handler)
  }, [zoom])

  const reset = () => setView({ s: 1, x: 0, y: 0 })

  const endDrag = (e: React.PointerEvent<HTMLImageElement>) => {
    drag.current = null
    setDragging(false)
    if (e.currentTarget.hasPointerCapture(e.pointerId)) {
      e.currentTarget.releasePointerCapture(e.pointerId)
    }
  }

  return createPortal(
    <div
      ref={overlayRef}
      className="fixed inset-0 z-50 flex items-center justify-center overflow-hidden bg-black/80 p-4"
      onClick={onClose}
    >
      {failed ? (
        <BrokenImage className="bg-white/10" />
      ) : (
        <img
          src={src}
          alt=""
          referrerPolicy="no-referrer"
          draggable={false}
          style={{
            transform: `translate(${view.x}px, ${view.y}px) scale(${view.s})`,
            cursor: dragging ? 'grabbing' : view.s > 1 ? 'grab' : 'zoom-in',
            touchAction: 'none',
          }}
          className="max-h-[86vh] max-w-[92vw] select-none rounded-xl object-contain"
          onClick={(e) => e.stopPropagation()}
          onDoubleClick={() => (view.s > 1 ? reset() : zoom(2))}
          onPointerDown={(e) => {
            if (view.s <= 1) return
            e.currentTarget.setPointerCapture(e.pointerId)
            drag.current = { x: e.clientX, y: e.clientY, vx: view.x, vy: view.y }
            setDragging(true)
          }}
          onPointerMove={(e) => {
            const d = drag.current
            if (!d) return
            setView((v) => ({ ...v, x: d.vx + (e.clientX - d.x), y: d.vy + (e.clientY - d.y) }))
          }}
          onPointerUp={endDrag}
          onPointerCancel={endDrag}
          onError={() => setFailed(true)}
        />
      )}

      {!failed && (
        <div
          className="absolute bottom-5 left-1/2 flex -translate-x-1/2 items-center gap-1 rounded-full bg-black/60 px-2 py-1.5 text-white backdrop-blur"
          onClick={(e) => e.stopPropagation()}
        >
          <button
            type="button"
            aria-label="缩小"
            onClick={() => zoom(1 / 1.25)}
            disabled={view.s <= 1}
            className="grid size-8 place-items-center rounded-full hover:bg-white/15 disabled:opacity-40"
          >
            <ZoomOut className="size-4" />
          </button>
          <button
            type="button"
            aria-label="重置缩放"
            onClick={reset}
            className="min-w-[3.5rem] rounded-full px-2 py-1 text-center font-mono text-[12px] tabular-nums hover:bg-white/15"
          >
            {Math.round(view.s * 100)}%
          </button>
          <button
            type="button"
            aria-label="放大"
            onClick={() => zoom(1.25)}
            disabled={view.s >= 8}
            className="grid size-8 place-items-center rounded-full hover:bg-white/15 disabled:opacity-40"
          >
            <ZoomIn className="size-4" />
          </button>
        </div>
      )}

      <a
        href={src}
        target="_blank"
        rel="noreferrer"
        onClick={(e) => e.stopPropagation()}
        className="absolute right-16 top-4 rounded-full bg-white/15 px-3 py-1.5 text-[12px] text-white hover:bg-white/25"
      >
        新标签打开
      </a>
      <button
        type="button"
        onClick={onClose}
        aria-label="关闭"
        className="absolute right-4 top-4 grid size-9 place-items-center rounded-full bg-white/15 text-white hover:bg-white/25"
      >
        <X className="size-5" />
      </button>
    </div>,
    document.body,
  )
}
