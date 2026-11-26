import { useId, useState } from 'react'
import { Chart } from '@gravity-ui/charts'
import type {
  ChartData,
  ChartInteractionItem,
  LineSeries,
  LineSeriesData,
} from '@gravity-ui/charts'
import './series-chart.css'

export interface ChartPoint {
  point_start: string
  value: number | null
  detail?: string
}

export interface ChartSeries {
  id: string
  metric_id: string
  name: string
  points: readonly ChartPoint[]
}

export interface ChartPointSelection {
  series_id: ChartSeries['id']
  metric_id: string
  point_start: string
}

export interface SeriesChartProps {
  title: string
  summary: string
  unit: string
  timeZone: string
  series: readonly ChartSeries[]
  pointId?: (point: ChartPointSelection) => string
  restoredSeriesId?: string
  onPointSelect: (point: ChartPointSelection) => void
}

const markers = [
  { symbol: 'circle', glyph: '●' },
  { symbol: 'diamond', glyph: '◆' },
  { symbol: 'square', glyph: '■' },
  { symbol: 'triangle', glyph: '▲' },
  { symbol: 'triangle-down', glyph: '▼' },
] as const
const maxSeries = markers.length

type PlotPoint = ChartPointSelection & { details: string }
type LineInteraction = ChartInteractionItem<LineSeriesData<PlotPoint>, 'line'>
const number = new Intl.NumberFormat('ru-RU', { maximumFractionDigits: 20 })

export function SeriesChart({
  title,
  summary,
  unit,
  timeZone,
  series,
  onPointSelect,
  pointId,
  restoredSeriesId,
}: SeriesChartProps) {
  const titleId = useId()
  const [chosenIds, setChosenIds] = useState<string[]>()
  const [selected, setSelected] = useState<{ id: string; start: string }>()
  const chosen = series.filter(({ id }) => chosenIds?.includes(id))
  const useDefaultSelection =
    !chosenIds ||
    series.length <= 1 ||
    (chosenIds.length > 0 && chosen.length === 0)
  const defaults = series.slice(0, maxSeries)
  const restored = series.find(({ id }) => id === restoredSeriesId)
  if (restored && !defaults.includes(restored))
    defaults.splice(maxSeries - 1, 1, restored)
  const visible = useDefaultSelection ? defaults : chosen.slice(0, maxSeries)
  const time = new Intl.DateTimeFormat('ru-RU', {
    dateStyle: 'medium',
    timeStyle: 'medium',
    timeZone,
  })
  const formatDate = (start: string) => time.format(new Date(start))
  const formatValue = (value: number | null) =>
    value === null ? 'Не рассчитано' : number.format(value)
  const pointDetails = (
    name: string,
    start: string,
    value: number | null,
    detail?: string,
  ) =>
    `${name}, ${formatDate(start)}, ${formatValue(value)}${value === null ? '' : ` ${unit}`}${detail ? `, ${detail}` : ''}`
  const hasPoints = visible.some(({ points }) => points.length > 0)
  const hasValues = visible.some(({ points }) =>
    points.some(({ value }) => value !== null),
  )
  const selectPoint = ({
    series_id,
    metric_id,
    point_start,
  }: ChartPointSelection) => {
    setSelected({ id: series_id, start: point_start })
    onPointSelect({ series_id, metric_id, point_start })
  }
  const pointsByPeriod = new Map<number, PlotPoint[]>()
  const lineSeries = visible.map(
    ({ id, metric_id, name, points }, index): LineSeries<PlotPoint> => ({
      type: 'line',
      name,
      lineWidth: 2,
      cursor: 'pointer',
      marker: {
        enabled: true,
        radius: 3,
        symbol: (markers[index] ?? markers[0]).symbol,
      },
      data: points
        .map(({ point_start, value, detail }) => {
          const x = Date.parse(point_start)
          const details = pointDetails(name, point_start, value, detail)
          const custom = { series_id: id, metric_id, point_start, details }
          const periodPoints = pointsByPeriod.get(x)
          if (periodPoints) periodPoints.push(custom)
          else pointsByPeriod.set(x, [custom])
          return {
            x,
            y: value,
            custom,
            ...(selected?.id === id && selected.start === point_start
              ? { annotation: { label: { text: details } } }
              : {}),
          }
        })
        .sort((a, b) => a.x - b.x),
    }),
  )
  const periodDetails = (start: string) =>
    pointsByPeriod
      .get(Date.parse(start))
      ?.map(({ series_id, details }) => <li key={series_id}>{details}</li>)
  const chart: ChartData<PlotPoint, LineInteraction> = {
    chart: {
      events: {
        click: ({ chart: api, position }) => {
          const point =
            position && api?.getClosest(position, { mode: 'tooltip' })?.point
          if (point?.custom) selectPoint(point.custom)
        },
      },
    },
    xAxis: {
      type: 'datetime',
      title: { text: 'Начало периода (UTC)' },
      labels: { style: { fontSize: '14px' } },
    },
    yAxis: [
      {
        title: { text: unit },
        labels: { style: { fontSize: '14px' } },
      },
    ],
    legend: { enabled: false },
    tooltip: {
      renderer: ({ hovered }) => {
        const point = (hovered.find(({ closest }) => closest) ?? hovered[0])
          ?.data.custom
        return (
          <ul className="cp-series-chart__tooltip">
            {point && periodDetails(point.point_start)}
          </ul>
        )
      },
    },
    series: { data: lineSeries },
  }

  return (
    <section className="cp-series-chart" aria-labelledby={titleId}>
      <h2 id={titleId}>{title}</h2>
      <p>{summary}</p>
      <p>
        Единица измерения: {unit}. Часовой пояс: {timeZone}.
      </p>
      {series.length > 1 && (
        <fieldset>
          <legend>Серии (до {maxSeries} одновременно)</legend>
          {series.map(({ id, name }) => {
            const index = visible.findIndex((item) => item.id === id)
            const checked = index >= 0
            return (
              <label key={id}>
                <input
                  type="checkbox"
                  checked={checked}
                  disabled={!checked && visible.length === maxSeries}
                  onChange={() => {
                    const ids = visible.map((item) => item.id)
                    setChosenIds(
                      checked ? ids.filter((key) => key !== id) : [...ids, id],
                    )
                  }}
                />
                <span aria-hidden="true">{markers[index]?.glyph}</span>
                {name}
              </label>
            )
          })}
        </fieldset>
      )}
      {hasPoints ? (
        <>
          {hasValues ? (
            <div className="cp-series-chart__plot" aria-hidden="true">
              <Chart data={chart} lang="ru" />
            </div>
          ) : (
            <p role="status">Нет рассчитанных значений для выбранных серий</p>
          )}
          <div
            className="cp-series-chart__table"
            role="region"
            aria-label="Таблица точек графика"
            tabIndex={0}
          >
            <table>
              <caption>Точки графика</caption>
              <thead>
                <tr>
                  <th scope="col">Серия</th>
                  <th scope="col">Начало периода</th>
                  <th scope="col">Значение ({unit})</th>
                </tr>
              </thead>
              <tbody>
                {visible.flatMap(
                  ({ id, metric_id, name, points }, seriesIndex) =>
                    points.map(({ point_start, value, detail }, rowIndex) => {
                      const date = formatDate(point_start)
                      const display = formatValue(value)
                      const details = pointDetails(
                        name,
                        point_start,
                        value,
                        detail,
                      )
                      const detailsId = `${titleId}-${String(seriesIndex)}-${String(rowIndex)}`
                      const active =
                        selected?.id === id && selected.start === point_start
                      return (
                        <tr key={JSON.stringify([id, point_start])}>
                          <td>{name}</td>
                          <td>
                            <button
                              id={pointId?.({
                                series_id: id,
                                metric_id,
                                point_start,
                              })}
                              type="button"
                              aria-label={details}
                              aria-describedby={detailsId}
                              aria-pressed={active}
                              onClick={() => {
                                selectPoint({
                                  series_id: id,
                                  metric_id,
                                  point_start,
                                })
                              }}
                            >
                              <time dateTime={point_start}>{date}</time>
                            </button>
                            <ul
                              id={detailsId}
                              className="cp-series-chart__details"
                            >
                              {periodDetails(point_start)}
                            </ul>
                          </td>
                          <td>
                            {display}
                            {detail && <div>{detail}</div>}
                          </td>
                        </tr>
                      )
                    }),
                )}
              </tbody>
            </table>
          </div>
        </>
      ) : (
        <p role="status">Нет точек для выбранных серий</p>
      )}
    </section>
  )
}
