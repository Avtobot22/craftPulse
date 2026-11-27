import { Text } from '@gravity-ui/uikit'
import { useLocation, useOutletContext, useSearchParams } from 'react-router'
import type { ApiClient } from '@/shared/api'
import type { CurrentUser } from '@/shared/auth'
import { periodError, readReportParams, reportQuery } from '@/shared/config'
import type { TeamReportScope } from '@/shared/config'
import { InspectionBack } from '@/shared/ui'
import {
  queueSearchForUpdate,
  readGroup,
  readQueueHistory,
  readQueuePage,
  readSort,
  waitGroups,
  waitSorts,
  writeQueuePage,
} from '../model/queue-search'
import type { WaitGroup } from '../model/queue-search'
import { QueueGroup } from './queue-group'
import { ReviewReport } from './review-report'
import './merge-request-queue.css'

const groups: readonly WaitGroup[] = ['no_reviewer', 'awaiting_first_response']

export function MergeRequestQueuePage({ api }: { api: ApiClient }) {
  const { project, team, actions } = useOutletContext<
    TeamReportScope & CurrentUser
  >()
  const [search, setSearch] = useSearchParams()
  const state: unknown = useLocation().state
  const group = readGroup(search.get('review_wait'))
  const sort = readSort(search.get('sort'))
  const params = readReportParams(search)
  const invalidPeriod = periodError(params)

  // Keep the originating task one history entry behind the queue.
  const fromInspection = Boolean(
    state &&
    typeof state === 'object' &&
    'fromInspection' in state &&
    state.fromInspection === true,
  )
  const queueHistory = {
    no_reviewer: readQueueHistory(state, 'no_reviewer'),
    awaiting_first_response: readQueueHistory(state, 'awaiting_first_response'),
  }

  function changeGroup(value: string) {
    const next = queueSearchForUpdate(search)
    if (value) next.set('review_wait', value)
    else next.delete('review_wait')
    setSearch(next, {
      replace: fromInspection,
      state: { fromInspection, queueHistory },
    })
  }

  function changeSort(value: string) {
    const next = queueSearchForUpdate(search)
    next.set('sort', value)
    for (const kind of groups) {
      writeQueuePage(next, kind, '', String(readQueuePage(next, kind).limit))
    }
    setSearch(next, { replace: fromInspection, state: { fromInspection } })
  }

  return (
    <section className="cp-mr-queue" aria-labelledby="mr-queue-heading">
      <InspectionBack />
      <Text as="h1" id="mr-queue-heading" variant="header-1">
        Очередь MR
      </Text>
      {!project ? (
        <p>Выберите проект в области просмотра.</p>
      ) : (
        <>
          <div className="cp-mr-queue__controls">
            <label>
              Вид ожидания
              <select
                value={group ?? ''}
                onChange={(event) => {
                  changeGroup(event.target.value)
                }}
              >
                <option value="">Обе группы</option>
                {groups.map((kind) => (
                  <option key={kind} value={kind}>
                    {waitGroups[kind]}
                  </option>
                ))}
              </select>
            </label>
            <label>
              Сортировка
              <select
                value={sort}
                onChange={(event) => {
                  changeSort(event.target.value)
                }}
              >
                {Object.entries(waitSorts).map(([value, label]) => (
                  <option key={value} value={value}>
                    {label}
                  </option>
                ))}
              </select>
            </label>
          </div>
          {groups
            .filter((kind) => !group || kind === group)
            .map((kind) => {
              const { cursor, limit } = readQueuePage(search, kind)
              return (
                <QueueGroup
                  key={kind}
                  api={api}
                  params={{
                    project_id: project.id,
                    state: 'open',
                    review_wait: kind,
                    sort,
                    limit,
                    ...(cursor ? { cursor } : {}),
                  }}
                  previous={cursor ? queueHistory[kind] : []}
                  goTo={(nextCursor, previous, nextLimit) => {
                    const next = queueSearchForUpdate(search)
                    writeQueuePage(next, kind, nextCursor, nextLimit)
                    setSearch(next, {
                      replace: fromInspection,
                      state: {
                        fromInspection,
                        queueHistory: { ...queueHistory, [kind]: previous },
                      },
                    })
                  }}
                />
              )
            })}
          <section
            className="cp-mr-queue__group"
            aria-labelledby="mr-review-report"
          >
            <h2 id="mr-review-report">Полученные ответы на ревью</h2>
            {!actions.includes('report.read') ? (
              <p role="status">Отчёт команды недоступен.</p>
            ) : !team ? (
              <p>Выберите команду, чтобы увидеть показатели за период.</p>
            ) : invalidPeriod ? (
              <p role="alert">{invalidPeriod}</p>
            ) : (
              <ReviewReport
                api={api}
                teamId={team.id}
                params={reportQuery(
                  { ...params, taskType: '', size: '', groupBy: '' },
                  project.id,
                )}
              />
            )}
          </section>
        </>
      )}
    </section>
  )
}
