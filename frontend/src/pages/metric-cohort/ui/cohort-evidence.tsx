import { Button } from '@gravity-ui/uikit'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { ActivityTime, EventRow } from '@/entities/activity'
import { isRevisionAccessDenied } from '@/entities/metric'
import { RestrictedResults } from '@/entities/result-access'
import { getTask, taskEventPages } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { ErrorState, LoadingState } from '@/shared/ui'
import type { CohortItem } from '@/shared/api'

export function ContributionInterval({
  interval,
}: {
  interval: NonNullable<Extract<CohortItem, { deleted: false }>['interval']>
}) {
  return (
    <dl>
      <dt>Начало</dt>
      <dd>
        <ActivityTime instant={interval.from} />
      </dd>
      <dt>Завершение</dt>
      <dd>
        {interval.to ? (
          <ActivityTime instant={interval.to} />
        ) : (
          'Открыт на дату среза'
        )}
      </dd>
    </dl>
  )
}

export function CohortEvidence({
  api,
  taskId,
  asOf,
  publishedAt,
}: {
  api: ApiClient
  taskId: string
  asOf: string
  publishedAt: string | null
}) {
  const task = useQuery({ ...getTask(api, taskId), gcTime: 0 })
  const history = useInfiniteQuery({
    ...taskEventPages(api, taskId),
    enabled: task.isSuccess,
    gcTime: 0,
  })

  if (
    isRevisionAccessDenied(task.error) ||
    isRevisionAccessDenied(history.error)
  )
    return <p role="status">История задачи недоступна.</p>

  if (task.isPending) return <LoadingState />
  if (task.isError)
    return (
      <ErrorState
        reason="Не удалось загрузить задачу"
        onRetry={() => void task.refetch()}
        retrying={task.isFetching}
      />
    )

  const cutoff = Date.parse(asOf)
  const publication = publishedAt === null ? Infinity : Date.parse(publishedAt)
  const blocks = task.data.blocked_intervals.filter(
    ({ from }) => Date.parse(from) <= cutoff,
  )
  const events = new Map(
    history.data?.pages
      .flatMap((page) => page.data)
      .filter(
        (event) =>
          Date.parse(event.event_time) <= cutoff &&
          Date.parse(event.observed_at) <= publication,
      )
      .map((event) => [event.id, event]),
  )
  const statusChanges = [...events.values()]
    .filter((event) => event.field === 'status' && event.after_value !== null)
    .sort(
      (left, right) =>
        Date.parse(left.event_time) - Date.parse(right.event_time),
    )
  return (
    <>
      <RestrictedResults
        redactions={task.data.redactions}
        access_limited={
          history.data?.pages.some((page) => page.access_limited) ?? false
        }
      />
      <h4>Блокировки задачи</h4>
      <p>По данным источника на дату среза.</p>
      {blocks.length > 0 ? (
        <ul>
          {blocks.map(({ from, to }, index) => (
            <li key={index}>
              <ContributionInterval
                interval={{
                  from,
                  to: to && Date.parse(to) <= cutoff ? to : null,
                }}
              />
            </li>
          ))}
        </ul>
      ) : (
        <p>Зафиксированных блокировок до даты среза нет.</p>
      )}
      {!history.isError && statusChanges.length > 0 && (
        <>
          <h4>Интервалы статусов по событиям</h4>
          <ul>
            {statusChanges.map((event, index) => (
              <li key={event.id}>
                {event.after_value}
                <ContributionInterval
                  interval={{
                    from: event.event_time,
                    to: statusChanges[index + 1]?.event_time ?? null,
                  }}
                />
              </li>
            ))}
          </ul>
        </>
      )}
      <h4>События задачи</h4>
      <p>События до даты среза, полученные до публикации ревизии.</p>
      {history.isPending ? (
        <LoadingState />
      ) : history.isError ? (
        <ErrorState
          reason="Не удалось загрузить события задачи"
          onRetry={() =>
            void (history.isFetchNextPageError
              ? history.fetchNextPage()
              : history.refetch())
          }
          retrying={history.isFetching}
        />
      ) : (
        <>
          {events.size ? (
            <ol>
              {[...events.values()].map((event) => (
                <li key={event.id}>
                  <EventRow event={event} />
                </li>
              ))}
            </ol>
          ) : (
            <p>На загруженных страницах событий до даты среза нет.</p>
          )}
          {history.hasNextPage && (
            <Button
              size="xl"
              loading={history.isFetching}
              onClick={() => void history.fetchNextPage()}
            >
              Загрузить ещё события
            </Button>
          )}
        </>
      )}
    </>
  )
}
