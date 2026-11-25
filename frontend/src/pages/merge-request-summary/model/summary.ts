import { queryOptions } from '@tanstack/react-query'
import { getMergeRequestSummary, InvalidResponse } from '@/shared/api'
import type { ApiClient } from '@/shared/api'

export function getSummaryQuery(
  api: ApiClient,
  mr: string,
  revision: number | undefined,
  head: string | undefined,
) {
  const revisionParams = revision === undefined ? {} : { revision_no: revision }
  if (head === undefined) return getMergeRequestSummary(api, mr, revisionParams)

  const options = getMergeRequestSummary(api, mr, {
    ...revisionParams,
    head_sha: head,
  })
  return queryOptions({
    ...options,
    queryKey: [...options.queryKey, head],
    select: (summary) => {
      if (summary.head_sha !== head) throw new InvalidResponse()
      return summary
    },
  })
}
