import { useId, useState } from 'react'
import { useSearchParams } from 'react-router'
import { MetricValue } from '@/entities/metric'
import { RestrictedResults } from '@/entities/result-access'
import type { ApiClient, ReportSeries } from '@/shared/api'
import { reportSteps } from '../model/metric-details'
import { seriesKey, seriesTitle } from '../model/metrics'
import type { ReportBlockProps } from '../model/metrics'
import { useCrossCohort } from '../model/use-cross-cohort'
import { CrossTaskTable } from './cross-task-table'
import { DetailPoint, DetailsStatus, StepSelect } from './detail-point'

const metricIds = ['X01', 'X02']

export function CrossMetricsBlock({
  block,
  api,
  canReadTasks,
}: {
  block: ReportBlockProps
  api: ApiClient
  canReadTasks: boolean
}) {
  const [search] = useSearchParams()
  const [selected, setSelected] = useState(() =>
    block.focus && /^\["X0[12]"/u.test(block.focus.seriesId)
      ? block.focus.pointStart
      : (search.get('cross_point') ?? ''),
  )
  const steps = reportSteps(block, metricIds)
  const step = (steps.includes(selected) ? selected : steps[0]) ?? ''
  if (!step) return null

  return (
    <section
      className="cp-team-report__block"
      aria-labelledby="team-report-cross"
    >
      <h2 id="team-report-cross">Задачи, которым нужно внимание</h2>
      <StepSelect
        steps={steps}
        step={step}
        label="Дата кросс-показателей"
        timeZone={block.metadata?.timeZone ?? 'UTC'}
        onChange={setSelected}
      />
      {metricIds.flatMap((metric) =>
        block.report.metrics
          .filter((series) => series.metric_id === metric)
          .map((series) => {
            const point = series.points.find(
              ({ point_start }) => point_start === step,
            )
            return (
              point && (
                <CrossMetricPoint
                  key={seriesKey(series)}
                  api={api}
                  block={block}
                  series={series}
                  point={point}
                  canReadTasks={canReadTasks}
                />
              )
            )
          }),
      )}
    </section>
  )
}

function CrossMetricPoint({
  api,
  block,
  series,
  point,
  canReadTasks,
}: {
  api: ApiClient
  block: ReportBlockProps
  series: ReportSeries
  point: ReportSeries['points'][number]
  canReadTasks: boolean
}) {
  const query = useCrossCohort(api, block, series, point)
  const { value, point_start: step } = point
  const data =
    !query.isError && query.isFetchedAfterMount ? query.data : undefined
  const id = 'cross-' + encodeURIComponent(seriesKey(series))
  const count = new Intl.NumberFormat('ru-RU')
  const captionId = useId()
  const withoutMR =
    value.state === 'NOT_EVALUATED'
      ? undefined
      : value.excluded_count === 0
        ? 0
        : data?.withoutMR

  return (
    <section
      className="cp-team-report__panel cp-team-report__detail"
      aria-labelledby={captionId}
    >
      <h3 id={captionId}>{seriesTitle(series, block.groupBy)}</h3>
      <MetricValue
        value={value}
        showBreakdown
        missingLabel="С неизвестным состоянием"
        sampleLabel="Всего задач в работе (WIP)"
        renderValue={(display) => (
          <DetailPoint block={block} series={series} step={step} id={id}>
            {display}
          </DetailPoint>
        )}
      />
      <p>
        Без подтверждённого MR:{' '}
        {withoutMR === undefined ? 'Нет данных' : count.format(withoutMR)}
      </p>
      <DetailsStatus
        query={query}
        reason="Не удалось прочитать когорту точки"
      />
      <RestrictedResults access_limited={data?.limited === true} />
      {data && data.tasks.length > 0 && (
        <CrossTaskTable
          tasks={data.tasks}
          report={block.report}
          point={step}
          captionId={captionId}
          focusPrefix={id}
          canReadTasks={canReadTasks}
        />
      )}
    </section>
  )
}
