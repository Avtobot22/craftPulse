import { generatePath, useSearchParams } from 'react-router'
import { RestrictedResults } from '@/entities/result-access'
import type { ApiClient, CohortItem, ReportSeries } from '@/shared/api'
import { routePaths } from '@/shared/config'
import { InspectionLink, useInspectionReturn } from '@/shared/ui'
import type { ReportBlockProps } from '../model/metrics'
import {
  technicalGapLabel,
  technicalSignalLabel,
} from '../model/technical-checks'
import { useTechnicalCohort } from '../model/use-technical-cohort'
import { DetailsStatus } from './detail-point'
import { ReportTaskLink } from './report-task-link'

type VisibleItem = Extract<CohortItem, { deleted: false }>

function label(object: VisibleItem['object'], fallback: string) {
  return (
    [object.display_key, object.title].filter(Boolean).join(' · ') || fallback
  )
}

export function TechnicalCohort({
  api,
  block,
  series,
  point,
  gaps,
  canReadTasks,
  canReadMRs,
  focusPrefix,
}: {
  api: ApiClient
  block: ReportBlockProps
  series: ReportSeries
  point: string
  gaps: boolean
  canReadTasks: boolean
  canReadMRs: boolean
  focusPrefix: string
}) {
  const [search] = useSearchParams()
  const query = useTechnicalCohort(api, block, series, point, gaps, search)
  const data =
    !query.isError && query.isFetchedAfterMount ? query.data : undefined
  useInspectionReturn(data !== undefined)

  return (
    <>
      <DetailsStatus
        query={query}
        reason="Не удалось прочитать когорту точки"
      />
      {data && <RestrictedResults access_limited={data.limited} />}
      {data?.items.length === 0 && (
        <p role="status">
          {gaps
            ? 'В доступной когорте разрывов нет.'
            : 'В доступной когорте совпадений нет.'}
        </p>
      )}
      {data && data.items.length > 0 && (
        <div className="cp-team-report__table-scroll">
          <table>
            <caption>
              {gaps ? 'Разрывы выбранного шага' : 'Совпадения в когорте точки'}
            </caption>
            <thead>
              <tr>
                <th scope="col">Задачи</th>
                <th scope="col">MR</th>
                {series.metric_id === 'X04' && <th scope="col">Pipeline</th>}
                <th scope="col">{gaps ? 'Причина' : 'Совпавшие признаки'}</th>
              </tr>
            </thead>
            <tbody>
              {data.items.map((item) => (
                <TechnicalCohortRow
                  key={item.object.id}
                  item={item}
                  report={block.report}
                  point={point}
                  focusPrefix={focusPrefix + '-' + item.object.id}
                  gaps={gaps}
                  pipeline={series.metric_id === 'X04'}
                  canReadTasks={canReadTasks}
                  canReadMRs={canReadMRs}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  )
}

function TechnicalCohortRow({
  item,
  report,
  point,
  focusPrefix,
  gaps,
  pipeline,
  canReadTasks,
  canReadMRs,
}: {
  item: VisibleItem
  report: ReportBlockProps['report']
  point: string
  focusPrefix: string
  gaps: boolean
  pipeline: boolean
  canReadTasks: boolean
  canReadMRs: boolean
}) {
  const refs = [item.object, ...item.basis]
  const tasks = refs.filter(({ kind }) => kind === 'task')
  const mergeRequests = refs.filter(({ kind }) => kind === 'merge_request')
  const signals = [
    ...new Set(item.signals.map(technicalSignalLabel).filter(Boolean)),
  ]
  const [search] = useSearchParams()
  const summarySearch = new URLSearchParams()
  for (const key of ['org_slug', 'team_id', 'project_id']) {
    const value = search.get(key)
    if (value) summarySearch.set(key, value)
  }

  return (
    <tr>
      <td>
        {tasks.length ? (
          <ul>
            {tasks.map((task) => (
              <li key={task.id}>
                {canReadTasks ? (
                  <ReportTaskLink
                    report={report}
                    taskId={task.id}
                    point={point}
                    pointParam="technical_point"
                    focusId={`${focusPrefix}-task-${task.id}`}
                  >
                    {label(task, 'Задача')}
                  </ReportTaskLink>
                ) : (
                  label(task, 'Задача')
                )}
              </li>
            ))}
          </ul>
        ) : (
          'Нет доступных задач'
        )}
      </td>
      <td>
        {mergeRequests.length ? (
          <ul>
            {mergeRequests.map((mr) => (
              <li key={mr.id}>
                {canReadMRs ? (
                  <InspectionLink
                    id={`${focusPrefix}-mr-${mr.id}`}
                    to={{
                      pathname: generatePath(routePaths.mergeRequestSummary, {
                        mr: encodeURIComponent(mr.id),
                      }),
                      search: summarySearch.toString(),
                    }}
                  >
                    {label(mr, 'Сводка MR')}
                  </InspectionLink>
                ) : (
                  label(mr, 'MR')
                )}
              </li>
            ))}
          </ul>
        ) : (
          'Нет доступных MR'
        )}
      </td>
      {pipeline && <td>{label(item.object, 'Pipeline')}</td>}
      <td>
        {gaps
          ? technicalGapLabel(item.reason)
          : signals.join(', ') || 'Нет данных'}
      </td>
    </tr>
  )
}
