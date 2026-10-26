import { useCallback, useLayoutEffect, useRef, useState } from 'react'

/**
 * 让滚动容器始终停在最新内容处。
 *
 * 规则：
 * - 距底小于 threshold 时视为「跟随最新」，内容变化就自动贴底；
 * - 用户主动上滚看历史后不再打扰，滚回接近底部会自动恢复跟随；
 * - 用 ResizeObserver 盯住每个子元素，图片或富文本异步撑高后仍能贴底
 *   （只靠 useEffect 里设一次 scrollTop 是不够的——那会儿图还没加载完）。
 *
 * deps 变化即视为「有新内容」。返回的 ref 是回调 ref，元素首次挂载/卸载都能感知，
 * 所以条件渲染出来的滚动容器也能正确接管。
 */
export function useStickToBottom<T extends HTMLElement>(deps: unknown[], threshold = 80) {
  const [node, setNode] = useState<T | null>(null)
  const pinned = useRef(true)

  const stick = useCallback(() => {
    pinned.current = true
    if (node) node.scrollTop = node.scrollHeight
  }, [node])

  useLayoutEffect(() => {
    if (!node) return
    const box = node

    const toBottom = () => {
      box.scrollTop = box.scrollHeight
    }

    const observer = new ResizeObserver(() => {
      if (pinned.current) toBottom()
    })
    const observeChildren = () => {
      for (const child of Array.from(box.children)) observer.observe(child)
    }

    const mutation = new MutationObserver(() => {
      observeChildren()
      if (pinned.current) toBottom()
    })
    mutation.observe(box, { childList: true })

    const onScroll = () => {
      pinned.current = box.scrollHeight - box.scrollTop - box.clientHeight <= threshold
    }
    box.addEventListener('scroll', onScroll, { passive: true })

    observeChildren()
    return () => {
      observer.disconnect()
      mutation.disconnect()
      box.removeEventListener('scroll', onScroll)
    }
  }, [node, threshold])

  // 新内容落地后贴底；首次挂载也算（此时 pinned 默认 true）
  useLayoutEffect(() => {
    if (!node || !pinned.current) return
    node.scrollTop = node.scrollHeight
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [node, ...deps])

  return { ref: setNode, node, stick }
}
