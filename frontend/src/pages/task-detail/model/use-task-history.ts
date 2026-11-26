import { useInfiniteQuery } from '@tanstack/react-query'
import { useEffect, useMemo } from 'react'
import { taskEventPages } from '@/shared/api'
import type { ApiClient, TimelineEvent } from '@/shared/api'
import { missingObservationEvidence } from './observations'
import { blockStartEvent } from './task-evidence'
import type { TaskDetail } from './task-evidence'

const noEvents: readonly TimelineEvent[] = []

export function useTaskHistory(
  api: ApiClient,
  taskId: string,
  task: TaskDetail | undefined,
) {
  const history = useInfiniteQuery({
    ...taskEventPages(api, taskId),
    gcTime: 0,
    refetchOnMount: 'always',
    enabled: Boolean(task),
  })
  const pending =
    history.isPending || (history.isFetching && !history.isFetchedAfterMount)
  const pages = pending ? undefined : history.data?.pages
  const events = useMemo(() => {
    if (!pages) return noEvents
    const byId = new Map<string, TimelineEvent>()
    for (const page of pages) {
      for (const event of page.data) {
        if (!byId.has(event.id)) byId.set(event.id, event)
      }
    }
    return [...byId.values()].sort(
      (left, right) =>
        Date.parse(right.event_time) - Date.parse(left.event_time),
    )
  }, [pages])
  const { fetchNextPage, hasNextPage, isError, isFetching } = history
  const seekEvidence =
    task !== undefined &&
    hasNextPage &&
    !isError &&
    !isFetching &&
    canFindEarlierEvidence(task, events)

  useEffect(() => {
    if (seekEvidence) void fetchNextPage({ cancelRefetch: false })
  }, [fetchNextPage, seekEvidence, pages])

  return { history, pending, pages: pages ?? [], events }
}

function canFindEarlierEvidence(
  task: TaskDetail,
  events: readonly TimelineEvent[],
) {
  const oldest = events.at(-1)
  // Pages are newest first. An equal timestamp can straddle a cursor.
  const reaches = (instant: string) =>
    !oldest || Date.parse(instant) <= Date.parse(oldest.event_time)
  return (
    task.observations.some(
      (observation) =>
        observation.since !== null &&
        reaches(observation.since) &&
        missingObservationEvidence(observation, events),
    ) ||
    task.blocked_intervals.some(
      ({ from }) =>
        reaches(from) &&
        !blockStartEvent(task.id, from, events)?.object.source_url,
    )
  )
}
