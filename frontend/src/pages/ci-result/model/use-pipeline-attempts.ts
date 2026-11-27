import { useInfiniteQuery } from '@tanstack/react-query'
import { pipelineAttemptPages } from '@/shared/api'
import type { ApiClient } from '@/shared/api'

export function usePipelineAttempts(api: ApiClient, pipelineId: string) {
  return useInfiniteQuery(pipelineAttemptPages(api, pipelineId))
}
