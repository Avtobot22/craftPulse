import { Alert, Button, Text } from '@gravity-ui/uikit'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import { useLocation } from 'react-router'
import { ActivityTime } from '@/entities/activity'
import { RestrictedResults } from '@/entities/result-access'
import {
  ApiError,
  loadMetricRevision,
  projectTimelinePages,
} from '@/shared/api'
import type { ApiClient, ProjectTimelineParams } from '@/shared/api'
import {
  EmptyState,
  ErrorState,
  LoadingState,
  StaleState,
  UnavailableRedirect,
} from '@/shared/ui'
import { readTimelineReturn } from '../model/timeline-return'
import { TimelineCoverage, timelineCoverage } from './timeline-coverage'
import { TimelineEvents } from './timeline-events'

export function TimelineResults({
  api,
  projectId,
  params,
  canReadRevision,
}: {
  api: ApiClient
  projectId: string
  params: ProjectTimelineParams
  canReadRevision: boolean
}) {
  const heading = useRef<HTMLHeadingElement>(null)
  const query = useInfiniteQuery({
    ...projectTimelinePages(api, projectId, params),
    refetchOnMount: 'always',
  })
  const returnPosition = readTimelineReturn(useLocation().state)
  const pageCount = query.data?.pages.length ?? 0
  const { fetchNextPage, isFetching } = query
  const restoringPages =
    query.hasNextPage &&
    !query.isError &&
    pageCount < (returnPosition?.pageCount ?? 1)
  useEffect(() => {
    if (restoringPages && !isFetching) {
      void fetchNextPage({ cancelRefetch: false })
    }
  }, [fetchNextPage, isFetching, pageCount, restoringPages])
  const revisionId = params.revision_id ?? ''
  const revision = useQuery({
    queryKey: ['projects', 'timeline-revision', revisionId],
    queryFn: ({ signal }) => loadMetricRevision(api, revisionId, signal),
    enabled: canReadRevision && revisionId !== '',
    refetchOnMount: 'always',
  })

  const error = query.error instanceof ApiError ? query.error : null
  if (error?.kind === 'not-found' || error?.kind === 'forbidden') {
    return <UnavailableRedirect error={error} />
  }
  if (
    query.isPending ||
    (isFetching && !query.isFetchedAfterMount) ||
    restoringPages
  ) {
    return <LoadingState />
  }

  const stale =
    query.isError &&
    !query.isFetchNextPageError &&
    (error?.kind === 'network' || (error?.problem?.status ?? 0) >= 500)
  const page =
    query.isError && !query.isFetchNextPageError && !stale
      ? undefined
      : query.data?.pages[0]
  const correlationId = error?.problem?.correlation_id
  const snapshotAt = query.dataUpdatedAt
    ? new Date(query.dataUpdatedAt).toISOString()
    : ''
  const failure = query.isError && (
    <ErrorState
      reason={
        query.isFetchNextPageError
          ? 'Не удалось загрузить следующие события'
          : 'Не удалось загрузить таймлайн'
      }
      {...(correlationId ? { correlationId } : {})}
      {...(snapshotAt ? { lastSuccess: snapshotAt } : {})}
      retrying={query.isFetching}
      onRetry={() => {
        if (query.isFetchNextPageError)
          void query.fetchNextPage({ cancelRefetch: false })
        else void query.refetch()
      }}
    />
  )
  if (!page) return failure

  const events = query.data?.pages.flatMap(({ data }) => data) ?? []
  const limited =
    query.data?.pages.some(
      ({ access_limited, access_notes }) =>
        access_limited || access_notes.length > 0,
    ) ?? false
  const { historyMissing, historyUncertain, noSources } = timelineCoverage(
    page,
    params.from,
  )
  const emptyTitle = historyMissing
    ? 'История за выбранный период не загружена'
    : historyUncertain
      ? 'Полнота истории за выбранный период неизвестна'
      : limited
        ? 'Нет доступных событий за выбранный период'
        : noSources
          ? 'Источники проекта не настроены'
          : 'Нет событий за выбранный период'
  const snapshot =
    canReadRevision && revision.isSuccess && revision.isFetchedAfterMount
      ? revision.data
      : undefined
  const results = (
    <>
      {revisionId && (
        <Alert
          theme="info"
          title="Срез знания по ревизии"
          message={
            snapshot ? (
              <>
                Дата ревизии: <ActivityTime instant={snapshot.as_of} />
              </>
            ) : canReadRevision && revision.isFetching ? (
              'Загрузка даты ревизии'
            ) : (
              'Дата ревизии недоступна'
            )
          }
        />
      )}
      <TimelineCoverage page={page} from={params.from} to={params.to} />
      <RestrictedResults access_limited={limited} />
      <section
        className="cp-timeline__results"
        aria-labelledby="timeline-results"
      >
        <Text
          as="h2"
          variant="header-2"
          id="timeline-results"
          ref={heading}
          tabIndex={-1}
        >
          События
        </Text>
        {events.length ? (
          <TimelineEvents
            events={events}
            projectId={projectId}
            params={params}
            returnPosition={returnPosition}
            pageCount={pageCount}
          />
        ) : (
          !query.hasNextPage && (
            <EmptyState
              reason={
                historyMissing || historyUncertain
                  ? 'history'
                  : noSources
                    ? 'source'
                    : 'filters'
              }
              title={emptyTitle}
            />
          )
        )}
        {events.length > 0 && (
          <Text as="p" role="status">
            Получено: {events.length}
          </Text>
        )}
        {!stale && failure}
        {!stale && query.hasNextPage && !query.isFetchNextPageError && (
          <Button
            size="xl"
            disabled={query.isFetching}
            onClick={() => {
              heading.current?.focus()
              void query.fetchNextPage({ cancelRefetch: false })
            }}
          >
            {query.isFetchingNextPage ? 'Загрузка' : 'Ещё'}
          </Button>
        )}
      </section>
    </>
  )
  return stale ? (
    <StaleState snapshotAt={snapshotAt} currentError={failure}>
      {results}
    </StaleState>
  ) : (
    results
  )
}
