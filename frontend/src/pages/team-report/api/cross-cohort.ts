import { InvalidResponse, loadCohort } from '@/shared/api'
import type { ApiClient, CohortItem, CohortQuery } from '@/shared/api'

type VisibleItem = Extract<CohortItem, { deleted: false }>

export async function loadCrossCohort(
  api: ApiClient,
  revisionId: string,
  query: CohortQuery,
  taskType: string | null | undefined,
  signal: AbortSignal,
) {
  const items: VisibleItem[] = []
  const seen = new Set<string>()
  let cursor: string | undefined
  let limited = false
  do {
    const page = await loadCohort(
      api,
      revisionId,
      { ...query, ...(cursor ? { cursor } : {}) },
      signal,
    )
    limited ||= page.accessLimited || page.accessNotes.length > 0
    for (const item of page.items) {
      if (item.deleted) {
        limited = true
      } else if (
        item.object.kind === 'task' &&
        item.role === query.role &&
        (taskType === undefined || (item.taskType ?? null) === taskType)
      ) {
        items.push(item)
      }
    }
    cursor = page.next ?? undefined
    if (cursor) {
      if (seen.has(cursor)) throw new InvalidResponse()
      seen.add(cursor)
    }
  } while (cursor)
  return { items, limited }
}
