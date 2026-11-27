import { useId } from 'react'
import { useSearchParams } from 'react-router'
import { Button } from '@gravity-ui/uikit'
import { MetricValue } from '@/entities/metric'
import type { ApiClient, ReportSeries } from '@/shared/api'
import { reportSteps } from '../model/metric-details'
import type { ReportBlockProps } from '../model/metrics'
import { seriesKey } from '../model/metrics'
import {
  chainStages,
  technicalMetrics,
  technicalSeriesLabel,
} from '../model/technical-checks'
import { DetailPoint, StepSelect } from './detail-point'
import { TechnicalCohort } from './technical-cohort'

interface Props {
  block: ReportBlockProps
  api: ApiClient
  canReadTasks: boolean
  canReadMRs: boolean
}

export function TechnicalChecksBlock(props: Props) {
  const { block } = props
  const [search, setSearch] = useSearchParams()
  const steps = reportSteps(block, Object.keys(technicalMetrics))
  const selected = block.focus?.targetId.startsWith('technical-value-')
    ? block.focus.pointStart
    : search.get('technical_point')
  const step = steps.find((start) => start === selected) ?? steps[0]
  if (!step) return null

  return (
    <section
      className="cp-team-report__block"
      aria-labelledby="team-report-technical"
    >
      <h2 id="team-report-technical">Проблемы текущих проверок</h2>
      <StepSelect
        steps={steps}
        step={step}
        label="Начало интервала технических показателей"
        timeZone={block.metadata?.timeZone ?? 'UTC'}
        onChange={(point) => {
          const next = new URLSearchParams(search)
          next.set('technical_point', point)
          setSearch(next, { replace: true, preventScrollReset: true })
        }}
      />
      <div className="cp-team-report__charts">
        {Object.entries(technicalMetrics).map(([id, metric]) => {
          const rows = block.report.metrics.filter(
            ({ metric_id, points }) =>
              metric_id === id &&
              points.some(({ point_start }) => point_start === step),
          )
          return (
            rows.length > 0 && (
              <TechnicalMetric
                key={id}
                {...props}
                step={step}
                rows={rows}
                metric={metric}
                metricId={id}
              />
            )
          )
        })}
      </div>
    </section>
  )
}

function TechnicalMetric({
  block,
  api,
  canReadTasks,
  canReadMRs,
  step,
  rows,
  metric,
  metricId,
}: Props & {
  step: string
  rows: ReportSeries[]
  metric: { title: string; first: string }
  metricId: string
}) {
  const [search, setSearch] = useSearchParams()
  const captionId = useId()
  const selectionParam = `technical_series_${metricId}`
  const focused = rows.find((row) => seriesKey(row) === block.focus?.seriesId)
  const requested = search.get(selectionParam)
  const series =
    focused ??
    rows.find((row) => seriesKey(row) === requested) ??
    rows.find(
      ({ group, estimator }) => (group?.signal ?? estimator) === metric.first,
    ) ??
    rows[0]
  const point = series?.points.find(({ point_start }) => point_start === step)
  if (!series || !point) return null

  const key = seriesKey(series)
  const id = 'technical-value-' + encodeURIComponent(key)
  const showCohort =
    search.get('technical_cohort') === key &&
    search.get('revision_id') === block.report.revision.id
  const gaps =
    metricId === 'X05' &&
    chainStages.includes(series.group?.signal ?? series.estimator ?? '')

  function select(value: string) {
    const next = new URLSearchParams(search)
    next.set(selectionParam, value)
    if (showCohort) next.set('technical_cohort', value)
    setSearch(next, { replace: true, preventScrollReset: true })
  }

  function toggleCohort() {
    const next = new URLSearchParams(search)
    if (showCohort) next.delete('technical_cohort')
    else {
      next.set('technical_cohort', key)
      next.set('technical_point', step)
      next.set(selectionParam, key)
      next.set('revision_id', block.report.revision.id)
    }
    setSearch(next, { replace: true, preventScrollReset: true })
  }

  return (
    <section
      className="cp-team-report__panel cp-team-report__detail"
      aria-labelledby={captionId}
    >
      <h3 id={captionId}>{metric.title}</h3>
      {rows.length > 1 ? (
        <label className="cp-team-report__point-period">
          {metricId === 'X05' ? 'Шаг цепочки или проверка' : 'Признак'}
          <select
            value={key}
            onChange={(event) => {
              select(event.currentTarget.value)
            }}
          >
            {rows.map((row) => (
              <option key={seriesKey(row)} value={seriesKey(row)}>
                {technicalSeriesLabel(row, block.groupBy)}
              </option>
            ))}
          </select>
        </label>
      ) : (
        <p>{technicalSeriesLabel(series, block.groupBy)}</p>
      )}
      <MetricValue
        value={point.value}
        showBreakdown
        missingLabel="С неизвестным состоянием"
        sampleLabel={metricId === 'X04' ? 'Всего pipeline' : 'Всего в выборке'}
        renderValue={(display) => (
          <DetailPoint
            block={block}
            series={series}
            step={step}
            id={id}
            openCohort
          >
            {display}
          </DetailPoint>
        )}
      />
      <Button onClick={toggleCohort} aria-expanded={showCohort}>
        {gaps ? 'Задачи с разрывами' : 'Задачи когорты'}
      </Button>
      {showCohort && (
        <TechnicalCohort
          api={api}
          block={block}
          series={series}
          point={step}
          gaps={gaps}
          canReadTasks={canReadTasks}
          canReadMRs={canReadMRs}
          focusPrefix={id}
        />
      )}
    </section>
  )
}
