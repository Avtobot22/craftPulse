import { Card } from '@gravity-ui/uikit'
import { useId } from 'react'
import type { ReactNode } from 'react'
import { MetricValue, formatValue, metricReason } from '@/entities/metric'
import { SeriesChart } from '@/shared/ui/series-chart'
import type { ChartPointSelection } from '@/shared/ui/series-chart'
import type { ReportSeries } from '../api/report-api'
import {
  formatTime,
  latestPoint,
  seriesKey,
  seriesTitle,
} from '../model/metrics'
import type { ReportBlockProps } from '../model/metrics'

const pointStates = {
  ASSESSED: 'Оценено',
  PROVISIONAL: 'Частичные данные',
  BOUNDED: 'Известна граница',
  NOT_EVALUATED: 'Не проверено',
  UNKNOWN: 'Неизвестно',
  NOT_APPLICABLE: 'Неприменимо',
}

function plotValue(value: ReportSeries['points'][number]['value']) {
  if (value.state !== 'ASSESSED' && value.state !== 'PROVISIONAL') return null
  if (['ratio', 'percent', '%'].includes(value.unit) && !value.denominator)
    return null
  return value.value
}

export function FlowChart({
  block,
  title,
  summary,
  metricIds,
  chartId,
}: {
  block: ReportBlockProps
  title: string
  summary: string
  metricIds: readonly string[]
  chartId: string
}) {
  const series = block.report.metrics.filter(({ metric_id }) =>
    metricIds.includes(metric_id),
  )
  if (!series.length) return <p>Нет рядов для графика «{title}».</p>
  const entries = new Map(series.map((item) => [seriesKey(item), item]))
  const pointId = ({ series_id, point_start }: ChartPointSelection) =>
    `chart-${chartId}-${encodeURIComponent(series_id)}-${point_start}`
  return (
    <Card className="cp-team-report__panel" view="outlined">
      <SeriesChart
        title={title}
        summary={summary}
        unit="шт."
        timeZone={block.metadata?.timeZone ?? 'UTC'}
        series={[...entries].map(([id, item]) => ({
          id,
          metric_id: item.metric_id,
          name: seriesTitle(item, block.groupBy),
          points: item.points.map(({ point_start, value }) => {
            const measured = plotValue(value)
            const blockedShare = item.metric_id === 'T13' && measured !== null
            return {
              point_start,
              value: blockedShare ? value.numerator : measured,
              detail: [
                pointStates[value.state],
                blockedShare &&
                  `Доля в WIP: ${formatValue(measured, value.unit)}`,
                value.state === 'BOUNDED' && value.bound
                  ? `${value.bound.kind === 'lower' ? 'Не менее' : 'Не более'} ${formatValue(value.bound.value, value.unit, value.bound.kind)}`
                  : '',
                value.n === null ? '' : `n = ${String(value.n)}`,
                metricReason(value.reason_code),
              ]
                .filter(Boolean)
                .join(', '),
            }
          }),
        }))}
        pointId={pointId}
        {...(block.focus?.chartSeries
          ? { restoredSeriesId: block.focus.chartSeries }
          : {})}
        onPointSelect={(point) => {
          const item = entries.get(point.series_id)
          if (item)
            block.selectPoint(
              item,
              point.point_start,
              pointId(point),
              point.series_id,
            )
        }}
      />
    </Card>
  )
}

export function MetricCard({
  block,
  series,
  point = latestPoint(series),
  children,
}: {
  block: ReportBlockProps
  series: ReportSeries
  point?: ReportSeries['points'][number]
  children?: ReactNode
}) {
  const titleId = useId()
  const title = seriesTitle(series, block.groupBy)
  const id = `metric-${encodeURIComponent(seriesKey(series))}-${point?.point_start ?? ''}`
  const timeZone = block.metadata?.timeZone ?? 'UTC'
  return (
    <Card
      className="cp-team-report__panel cp-team-report__metric"
      view="outlined"
      aria-labelledby={titleId}
    >
      <h3 id={titleId}>{title}</h3>
      {point ? (
        <>
          <time dateTime={point.point_start}>
            {formatTime(point.point_start, timeZone)}
          </time>
          <MetricValue
            value={point.value}
            renderValue={(display) => (
              <button
                type="button"
                className="cp-team-report__point-link"
                id={id}
                aria-label={`${title}: ${display}`}
                onClick={() => {
                  block.selectPoint(series, point.point_start, id)
                }}
              >
                {display}
              </button>
            )}
          />
        </>
      ) : (
        <p>Нет точек за этот период.</p>
      )}
      {children}
    </Card>
  )
}
