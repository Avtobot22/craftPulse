import { Card } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { formatValue, metricReason } from '@/entities/metric'
import type { ApiClient } from '@/shared/api'
import { loadTargetVersions } from '../api/metric-details'
import type { ReportSeries } from '../api/report-api'
import { groupLabel } from '../model/metric-details'
import { formatTime, seriesKey } from '../model/metrics'
import type { ReportBlockProps } from '../model/metrics'
import { DetailPoint, DetailsStatus } from './detail-point'

function displayOutcome(
  value: ReportSeries['points'][number]['value'],
  metricId: string,
) {
  const { numerator, denominator, state, unit, reason_code } = value
  if (
    value.value === null ||
    numerator === null ||
    denominator === null ||
    denominator <= 0
  ) {
    const reason =
      metricReason(reason_code) ??
      (metricId === 'T23'
        ? 'Нет задач с завершённым окном наблюдения'
        : 'Нет задач с известным началом, размером и целевым сроком')
    return `Не рассчитано · ${reason}`
  }
  const display =
    unit === 'count'
      ? `${String(numerator)} из ${String(denominator)}`
      : `${formatValue(value.value, unit)} (${String(numerator)} из ${String(denominator)})`
  return state === 'PROVISIONAL' ? `${display} · Частичные данные` : display
}

export function OutcomePoints({
  block,
  metricId,
  step,
}: {
  block: ReportBlockProps
  metricId: 'T10' | 'T11' | 'T23'
  step?: string
}) {
  const series = block.report.metrics.filter(
    ({ metric_id }) => metric_id === metricId,
  )
  return (
    <div className="cp-team-report__table-scroll">
      <table>
        <thead>
          <tr>
            <th scope="col">Начало шага</th>
            {block.groupBy && <th scope="col">Группа</th>}
            {metricId === 'T23' && <th scope="col">Окно наблюдения</th>}
            <th scope="col">
              {metricId === 'T23'
                ? 'Переоткрыто'
                : metricId === 'T10'
                  ? 'Уложились в срок'
                  : 'Превысили срок'}
            </th>
            <th scope="col">Неизвестно</th>
            {metricId === 'T23' && <th scope="col">Незрелые задачи</th>}
          </tr>
        </thead>
        <tbody>
          {series.flatMap((item) =>
            item.points
              .filter(
                ({ point_start }) => step === undefined || step === point_start,
              )
              .map(({ point_start, value }) => {
                const id = `outcome-${encodeURIComponent(seriesKey(item))}-${point_start}`
                const window =
                  block.metadata?.reopenWindows[item.definition_version]
                return (
                  <tr key={id}>
                    <th scope="row">
                      <time dateTime={point_start}>
                        {formatTime(
                          point_start,
                          block.metadata?.timeZone ?? 'UTC',
                        )}
                      </time>
                    </th>
                    {block.groupBy && (
                      <td>
                        {item.group
                          ? (item.group[
                              block.groupBy === 'task_type'
                                ? 'task_type'
                                : 'size_label'
                            ] ??
                            (block.groupBy === 'task_type'
                              ? 'Тип неизвестен'
                              : 'Без размера'))
                          : 'Все задачи'}
                      </td>
                    )}
                    {metricId === 'T23' && (
                      <td>
                        {window === undefined
                          ? 'Не указано'
                          : `${String(window)} дн. после первого завершения`}
                      </td>
                    )}
                    <td>
                      <DetailPoint
                        block={block}
                        series={item}
                        step={point_start}
                        id={id}
                      >
                        {displayOutcome(value, metricId)}
                      </DetailPoint>
                    </td>
                    <td>
                      <DetailPoint
                        block={block}
                        series={item}
                        step={point_start}
                        id={`${id}-unknown`}
                      >
                        {value.missing_count}
                      </DetailPoint>
                    </td>
                    {metricId === 'T23' && (
                      <td>
                        <DetailPoint
                          block={block}
                          series={item}
                          step={point_start}
                          id={`${id}-immature`}
                        >
                          {value.excluded_count}
                        </DetailPoint>
                      </td>
                    )}
                  </tr>
                )
              }),
          )}
        </tbody>
      </table>
    </div>
  )
}

function CurrentTargetVersions({
  block,
  api,
  canRead,
}: {
  block: ReportBlockProps
  api: ApiClient
  canRead: boolean
}) {
  const teamId = block.report.team_id
  const query = useQuery({
    queryKey: ['team-report', 'target-versions', teamId],
    queryFn: ({ signal }) => loadTargetVersions(api, teamId, signal),
    enabled: canRead,
  })
  const versions = new Map<string, NonNullable<typeof query.data>[number]>()
  for (const target of query.isError ? [] : (query.data ?? [])) {
    if (block.taskType && block.taskType !== target.taskType) continue
    if (block.size && block.size !== target.size) continue
    const key = JSON.stringify([target.taskType, target.size])
    if (target.version > (versions.get(key)?.version ?? 0))
      versions.set(key, target)
  }
  return (
    <Card
      className="cp-team-report__panel cp-team-report__detail"
      view="outlined"
    >
      <h3>Действующие целевые сроки команды</h3>
      <p>Доли по шагам рассчитаны по настройкам выбранной ревизии.</p>
      {!canRead ? (
        <p>Для просмотра версий нужен доступ к настройкам процесса команды.</p>
      ) : (
        <>
          <DetailsStatus
            query={query}
            reason="Не удалось прочитать версии сроков"
          />
          {versions.size > 0 ? (
            <div className="cp-team-report__table-scroll">
              <table>
                <thead>
                  <tr>
                    <th scope="col">Тип и размер</th>
                    <th scope="col">Версия</th>
                    <th scope="col">Срок</th>
                    <th scope="col">n основания</th>
                  </tr>
                </thead>
                <tbody>
                  {[...versions].map(([key, target]) => (
                    <tr key={key}>
                      <th scope="row">
                        {groupLabel(target.taskType, target.size)}
                      </th>
                      <td>{target.version}</td>
                      <td>{formatValue(target.duration, 'seconds')}</td>
                      <td>{target.n ?? 'Не указано'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            query.isSuccess && <p>Для выбранных групп сроки не заданы.</p>
          )}
        </>
      )}
    </Card>
  )
}

export function TargetComplianceBlock({
  block,
  api,
  canReadVersions,
}: {
  block: ReportBlockProps
  api: ApiClient
  canReadVersions: boolean
}) {
  if (
    !block.report.metrics.some(
      ({ metric_id, points }) => metric_id === 'T10' && points.length,
    )
  )
    return null
  return (
    <section
      className="cp-team-report__block"
      aria-labelledby="team-report-targets"
    >
      <h2 id="team-report-targets">Соблюдение целевых сроков</h2>
      <div className="cp-team-report__charts">
        <Card
          className="cp-team-report__panel cp-team-report__detail"
          view="outlined"
        >
          <p>
            Задачи без размера или целевого срока учитываются как неизвестные.
          </p>
          <OutcomePoints block={block} metricId="T10" />
        </Card>
        <CurrentTargetVersions
          block={block}
          api={api}
          canRead={canReadVersions}
        />
      </div>
    </section>
  )
}

export function ReopeningsBlock({ block }: { block: ReportBlockProps }) {
  if (
    !block.report.metrics.some(
      ({ metric_id, points }) => metric_id === 'T23' && points.length,
    )
  )
    return null
  return (
    <section
      className="cp-team-report__block"
      aria-labelledby="team-report-reopenings"
    >
      <h2 id="team-report-reopenings">Переоткрытия workflow</h2>
      <Card
        className="cp-team-report__panel cp-team-report__detail"
        view="outlined"
      >
        <p>
          Доля переоткрытий workflow среди задач с завершённым окном наблюдения.
          Незрелые задачи показаны отдельно и исключены из знаменателя.
        </p>
        <OutcomePoints block={block} metricId="T23" />
      </Card>
    </section>
  )
}
