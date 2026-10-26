import { useQuery } from '@tanstack/react-query'
import { get } from './api'
import { qk } from './query'
import type { UIConfig } from './types'

/** 管理台外观配置（公开接口，登录页也能读）。 */
export function useUIConfig() {
  return useQuery({
    queryKey: qk.ui,
    queryFn: () => get<UIConfig>('/ui'),
    staleTime: 60_000,
  })
}
