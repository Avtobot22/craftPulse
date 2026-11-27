import { useQuery } from '@tanstack/react-query'
import { useRef } from 'react'
import { Navigate } from 'react-router'
import { ActivityTime } from '@/entities/activity'
import { RestrictedResults } from '@/entities/result-access'
import { ApiError, listMergeRequests } from '@/shared/api'
import type { ApiClient, MergeRequestListParams } from '@/shared/api'
import { routePaths } from '@/shared/config'
import {
  CursorPagination,
  EmptyState,
  ErrorState,
  LoadingState,
  StaleState,
} from '@/shared/ui'
import { waitGroups } from '../model/queue-search'
import type { WaitGroup } from '../model/queue-search'
import { QueueRows } from './queue-rows'

export function QueueGroup({
  api,
  params,
  previous,
  goTo,
}: {
  api: ApiClient
  params: MergeRequestListParams & { review_wait: WaitGroup; limit: number }
  previous: string[]
  goTo: (cursor: string, previous: string[], limit: string) => void
}) {
  const heading = useRef<HTMLHeadingElement>(null)
  const { review_wait: kind, cursor = '', limit } = params
  const title = waitGroups[kind]
  const id = `mr-queue-${kind}`
  const query = useQuery({
    ...listMergeRequests(api, params),
    refetchOnMount: 'always',
  })
  const error = query.error instanceof ApiError ? query.error : undefined
  if (error?.kind === 'forbidden' || error?.kind === 'not-found') {
    return <Navigate to={routePaths.unavailable} replace />
  }
  const pending =
    query.isPending || (query.isFetching && !query.isFetchedAfterMount)
  const stale =
    query.isError &&
    (error?.kind === 'network' || (error?.problem?.status ?? 0) >= 500)
  const sourceLost = query.data?.access_notes.some(
    ({ code }) => code === 'source_access_lost',
  )
  const page =
    pending || (query.isError && (!stale || sourceLost))
      ? undefined
      : query.data
  const limited = page && (page.access_limited || page.access_notes.length > 0)
  const refreshedAt = new Date(query.dataUpdatedAt).toISOString()
  const correlationId = error?.problem?.correlation_id
  const failure = query.isError && (
    <ErrorState
      reason={`Не удалось загрузить группу «${title}»`}
      {...(correlationId ? { correlationId } : {})}
      onRetry={() => void query.refetch()}
      retrying={query.isFetching}
    />
  )
  const results = page && (
    <>
      <p>
        Получено <ActivityTime instant={refreshedAt} />
      </p>
      <RestrictedResults access_limited={Boolean(limited)} />
      {sourceLost && (
        <p role="status">Подключение к источнику потеряло доступ.</p>
      )}
      {page.data.length > 0 ? (
        <QueueRows requests={page.data} labelledBy={id} sort={params.sort} />
      ) : limited ? (
        <p role="status">На странице нет доступных MR.</p>
      ) : (
        <EmptyState reason="filters" title="Нет MR в ожидании" />
      )}
    </>
  )

  return (
    <section aria-labelledby={id} className="cp-mr-queue__group">
      <h2 id={id} ref={heading} tabIndex={-1}>
        {title}
      </h2>
      {pending ? (
        <LoadingState />
      ) : page && stale ? (
        <StaleState snapshotAt={refreshedAt} currentError={failure}>
          {results}
        </StaleState>
      ) : (
        (results ?? failure)
      )}
      <CursorPagination
        label={`Страницы группы «${title}»`}
        cursor={cursor}
        next={stale ? null : (page?.next_cursor ?? null)}
        previous={previous}
        limit={limit}
        received={limited ? undefined : page?.data.length}
        goTo={(next, history, size) => {
          heading.current?.focus()
          goTo(next, history, size)
        }}
      />
    </section>
  )
}
