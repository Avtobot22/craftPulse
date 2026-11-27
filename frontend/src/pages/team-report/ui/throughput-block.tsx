import { Button } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import type { ApiClient } from '@/shared/api'
import { ErrorState, LoadingState } from '@/shared/ui'
import { loadOutcomeReasons } from '../api/report-api'
import type { ReportSeries } from '@/shared/api'
import type { OutcomeFilters } from '../api/report-api'
import {
  formatTime,
  latestPoint,
  outcomeReasons,
  seriesKey,
} from '../model/metrics'
import type { ReportBlockProps } from '../model/metrics'
import { FlowChart, MetricCard } from './metric-block'

function OutcomeReasons({
  api,
  revision,
  point,
  filters,
  expectedCount,
}: {
  api: ApiClient
  revision: string
  point: string
  filters: OutcomeFilters
  expectedCount: number
}) {
  const [open, setOpen] = useState(false)
  const reasons = useQuery({
    queryKey: ['team-report', 'outcome-reasons', revision, point, filters],
    queryFn: ({ signal }) =>
      loadOutcomeReasons(api, revision, point, filters, signal),
    enabled: open,
  })
  const knownCount = reasons.data?.reduce(
    (total, [, count]) => total + count,
    0,
  )
  return (
    <>
      <Button
        size="xl"
        aria-expanded={open}
        onClick={() => {
          setOpen(!open)
        }}
      >
        {open ? 'Скрыть причины' : 'Показать причины'}
      </Button>
      {open &&
        (reasons.isError ? (
          <ErrorState
            reason="Не удалось прочитать причины закрытия"
            onRetry={() => void reasons.refetch()}
          />
        ) : reasons.isFetching || reasons.isPending ? (
          <LoadingState />
        ) : knownCount && knownCount <= expectedCount ? (
          <>
            <ul aria-label="Причины закрытия">
              {reasons.data.map(([code, count]) => (
                <li key={code}>
                  {outcomeReasons[code]}: {count}
                </li>
              ))}
            </ul>
            {knownCount < expectedCount && (
              <p>Причины доступны только для части закрытий.</p>
            )}
          </>
        ) : (
          <p>Нет доступных причин закрытия.</p>
        ))}
    </>
  )
}

function OutcomeCard({
  block,
  api,
  series,
}: {
  block: ReportBlockProps
  api: ApiClient
  series: ReportSeries
}) {
  const [selectedStart, setSelectedStart] = useState(() =>
    block.focus?.seriesId === seriesKey(series) ? block.focus.pointStart : '',
  )
  const point =
    series.points.find(({ point_start }) => point_start === selectedStart) ??
    latestPoint(series)
  const group = series.group
  const filters: OutcomeFilters = {
    projectIds: block.projectIds,
    taskType:
      group && block.groupBy === 'task_type'
        ? (group.task_type ?? null)
        : block.taskType,
    size:
      group && block.groupBy === 'size_label'
        ? (group.size_label ?? null)
        : block.size === 'unknown'
          ? null
          : block.size,
  }
  const timeZone = block.metadata?.timeZone ?? 'UTC'
  const outcomeCount = point?.value.value ?? point?.value.bound?.value ?? 0
  return (
    <MetricCard block={block} series={series} {...(point ? { point } : {})}>
      {point && series.points.length > 1 && (
        <label className="cp-team-report__point-period">
          Начало шага
          <select
            value={point.point_start}
            onChange={(event) => {
              setSelectedStart(event.currentTarget.value)
            }}
          >
            {series.points.map(({ point_start }) => (
              <option key={point_start} value={point_start}>
                {formatTime(point_start, timeZone)}
              </option>
            ))}
          </select>
        </label>
      )}
      {point && outcomeCount > 0 && !series.estimator && (
        <OutcomeReasons
          api={api}
          revision={block.report.revision.id}
          point={point.point_start}
          filters={filters}
          expectedCount={outcomeCount}
        />
      )}
    </MetricCard>
  )
}

export function ThroughputBlock({
  block,
  api,
}: {
  block: ReportBlockProps
  api: ApiClient
}) {
  const outcomes = block.report.metrics.filter(
    ({ metric_id }) => metric_id === 'T07',
  )
  return (
    <section
      className="cp-team-report__block"
      aria-labelledby="team-report-throughput"
    >
      <h2 id="team-report-throughput">Вход и выход задач</h2>
      <FlowChart
        block={block}
        chartId="throughput"
        title="Начало и завершение работы"
        summary="Первое начало работы и первое принятое завершение за каждый шаг."
        metricIds={['T04', 'T05']}
      />
      {outcomes.length > 0 && (
        <>
          <div className="cp-team-report__cards">
            {outcomes.map((series) => (
              <OutcomeCard
                key={seriesKey(series)}
                block={block}
                api={api}
                series={series}
              />
            ))}
          </div>
          <FlowChart
            block={block}
            chartId="outcomes"
            title="Отмены и дубликаты по шагам"
            summary="Эти закрытия показаны отдельно от впервые завершённых задач."
            metricIds={['T07']}
          />
        </>
      )}
    </section>
  )
}
