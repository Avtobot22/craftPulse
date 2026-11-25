import { generatePath, Link } from 'react-router'
import { ActivityTime, SourceLink } from '@/entities/activity'
import { MetricValue } from '@/entities/metric'
import type { TimelineEvent } from '@/shared/api'
import { routePaths } from '@/shared/config'
import type { TaskDetail } from '../model/task-evidence'
import { atEvidenceTime, isStatusChange } from '../model/task-evidence'

function Duration({
  seconds,
  revision,
  metric,
  missing,
}: {
  seconds: number | null
  revision: string | null
  metric?: 'T11' | 'T14' | 'T15'
  missing?: string
}) {
  if (seconds === null) return <p>{missing ?? 'Длительность неизвестна'}</p>
  return (
    <>
      <MetricValue
        value={{
          value: seconds,
          unit: 'seconds',
          state: 'ASSESSED',
          numerator: null,
          denominator: null,
          n: null,
          bound: null,
          low_sample: false,
          excluded_count: 0,
          missing_count: 0,
          reason_code: null,
        }}
      />
      {metric && revision && (
        <Link
          to={generatePath(routePaths.metricPassport, {
            revisionId: encodeURIComponent(revision),
            metricId: metric,
          })}
        >
          Как рассчитано
        </Link>
      )}
    </>
  )
}

export function TaskTiming({
  task,
  revision,
}: {
  task: TaskDetail
  revision: string | null
}) {
  const { target, over_target: overTarget, age_seconds: age } = task
  return (
    <section aria-labelledby="task-timing">
      <h2 id="task-timing" tabIndex={-1}>
        Возраст и срок
      </h2>
      <div className="cp-task-detail__timing">
        <div>
          <h3>Возраст текущего эпизода</h3>
          <Duration
            seconds={age}
            revision={revision}
            metric="T14"
            missing="Начало эпизода неизвестно"
          />
        </div>
        <div>
          <h3>Целевой срок</h3>
          {target === null ? (
            <p>Целевой срок не задан</p>
          ) : (
            <>
              <p>Версия {target.version_no}</p>
              <Duration
                seconds={target.duration_seconds}
                revision={revision}
                metric="T11"
              />
            </>
          )}
        </div>
        {target !== null && (
          <div>
            <h3>Превышение срока</h3>
            {overTarget === null || age === null ? (
              <p>Неизвестно</p>
            ) : overTarget ? (
              <Duration
                seconds={age - target.duration_seconds}
                revision={revision}
                metric="T11"
              />
            ) : (
              <p>Срок не превышен</p>
            )}
          </div>
        )}
      </div>
    </section>
  )
}

export function TaskBlocks({
  task,
  revision,
  events,
  receivedAt,
}: {
  task: TaskDetail
  revision: string | null
  events: readonly TimelineEvent[]
  receivedAt: number
}) {
  const { blocked_intervals: intervals } = task
  return (
    <section aria-labelledby="task-blocks">
      <h2 id="task-blocks">Блокировки</h2>
      {intervals.length === 0 ? (
        <p>Зафиксированных блокировок нет</p>
      ) : (
        <ul className="cp-task-detail__cards">
          {intervals.map(({ from, to }, index) => {
            const elapsed =
              ((to ? Date.parse(to) : receivedAt) - Date.parse(from)) / 1000
            const changes = events.filter(
              (event) =>
                atEvidenceTime(event, { kind: 'task', id: task.id }, from) &&
                isStatusChange(event),
            )
            const start = changes.length === 1 ? changes[0] : undefined
            return (
              <li key={`${from}-${String(index)}`}>
                <p>
                  Начало <ActivityTime instant={from} />
                </p>
                <p>
                  {to ? (
                    <>
                      Окончание <ActivityTime instant={to} />
                    </>
                  ) : (
                    'Открыта'
                  )}
                </p>
                <Duration
                  seconds={
                    Number.isFinite(elapsed) && elapsed >= 0 ? elapsed : null
                  }
                  revision={revision}
                  metric="T15"
                />
                {to === null && (
                  <p>
                    На момент загрузки{' '}
                    <ActivityTime
                      instant={new Date(receivedAt).toISOString()}
                    />
                  </p>
                )}
                {start?.object.source_url ? (
                  <SourceLink url={start.object.source_url}>
                    Событие начала блокировки в Jira DC
                  </SourceLink>
                ) : (
                  <p>Ссылка на событие начала недоступна</p>
                )}
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}
