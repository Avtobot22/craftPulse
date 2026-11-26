import { Button } from '@gravity-ui/uikit'
import { useState } from 'react'
import { ActivityTime, activitySourceName, EventRow } from '@/entities/activity'
import { RestrictedResults } from '@/entities/result-access'
import type { TimelineEvent, components, OpenCodes } from '@/shared/api'
import { EmptyState, ErrorState, LoadingState, StaleState } from '@/shared/ui'
import { ApiError } from '@/shared/api'

const actorKinds = {
  human: 'Человек',
  bot: 'Бот',
  system: 'Система',
  unknown: 'Неизвестен',
} satisfies Record<components['schemas']['ActorRef']['kind'], string>

export function TaskHistory({
  events,
  historyFrom,
  accessLimited,
  accessNotes,
  pending,
  fetching,
  more,
  error,
  snapshotAt,
  loadMore,
  retry,
}: {
  events: readonly TimelineEvent[]
  historyFrom: string | null
  accessLimited: boolean
  accessNotes: readonly OpenCodes<components['schemas']['AccessNote']>[]
  pending: boolean
  fetching: boolean
  more: boolean
  error: Error | null
  snapshotAt: number
  loadMore: () => void
  retry: () => void
}) {
  const [actor, setActor] = useState('')
  const visible =
    actor === '' ? events : events.filter((event) => event.actor.kind === actor)
  const correlationId =
    error instanceof ApiError ? error.problem?.correlation_id : undefined
  const failure = error && (
    <ErrorState
      reason="Не удалось загрузить историю задачи"
      {...(correlationId ? { correlationId } : {})}
      onRetry={retry}
      retrying={fetching}
    />
  )
  const history = (
    <>
      <RestrictedResults
        access_limited={accessLimited}
        access_notes={accessNotes}
      />
      {historyFrom && (
        <p>
          Граница истории: до <ActivityTime instant={historyFrom} /> часть
          событий может отсутствовать.
        </p>
      )}
      <label className="cp-task-detail__filter">
        Вид участника
        <select
          value={actor}
          onChange={(event) => {
            setActor(event.currentTarget.value)
          }}
        >
          <option value="">Все участники</option>
          {Object.entries(actorKinds).map(([kind, label]) => (
            <option key={kind} value={kind}>
              {label}
            </option>
          ))}
        </select>
      </label>
      {visible.length > 0 ? (
        <ol className="cp-task-detail__cards">
          {visible.map((event) => {
            const sourceName = activitySourceName(event.object.kind)
            return (
              <li key={event.id}>
                <EventRow
                  event={event}
                  {...(sourceName ? { sourceName } : {})}
                />
              </li>
            )
          })}
        </ol>
      ) : (
        <EmptyState
          reason={actor === '' ? 'history' : 'filters'}
          description={
            more
              ? 'На загруженных страницах таких событий нет. Можно загрузить ещё.'
              : 'Среди доступных событий совпадений нет.'
          }
          {...(actor === ''
            ? {}
            : {
                action: {
                  label: 'Сбросить фильтр',
                  onClick: () => {
                    setActor('')
                  },
                },
              })}
        />
      )}
      {more && (
        <Button
          size="l"
          loading={fetching}
          disabled={Boolean(failure)}
          onClick={loadMore}
        >
          Загрузить ещё
        </Button>
      )}
    </>
  )
  return (
    <section aria-labelledby="task-history">
      <h2 id="task-history">История задачи</h2>
      {pending ? (
        <LoadingState />
      ) : failure && !snapshotAt ? (
        failure
      ) : failure ? (
        <StaleState
          snapshotAt={new Date(snapshotAt).toISOString()}
          currentError={failure}
        >
          {history}
        </StaleState>
      ) : (
        history
      )}
    </section>
  )
}
