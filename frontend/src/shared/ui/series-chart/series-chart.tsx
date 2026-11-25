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
}

export interface ChartSeries {
  id: string
  metric_id: string
  name: string
  points: readonly ChartPoint[]
}

export interface ChartPointSelection {
  metric_id: string
  point_start: string
}

export interface SeriesChartProps {
  title: string
  summary: string
  unit: string
  timeZone: string
  series: readonly ChartSeries[]
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

type PlotPoint = ChartPointSelection & { seriesId: string; details: string }
type LineInteraction = ChartInteractionItem<LineSeriesData<PlotPoint>, 'line'>
const number = new Intl.NumberFormat('ru-RU', { maximumFractionDigits: 20 })

export function SeriesChart({
  title,
  summary,
  unit,
  timeZone,
  series,
  onPointSelect,
}: SeriesChartProps) {
  const titleId = useId()
  const [chosenIds, setChosenIds] = useState<string[]>()
  const [selected, setSelected] = useState<{ id: string; start: string }>()
  const chosen = series.filter(({ id }) => chosenIds?.includes(id))
  const useDefaultSelection =
    !chosenIds ||
    series.length <= 1 ||
    (chosenIds.length > 0 && chosen.length === 0)
  const visible = (useDefaultSelection ? series : chosen).slice(0, maxSeries)
  const time = new Intl.DateTimeFormat('ru-RU', {
    dateStyle: 'medium',
    timeStyle: 'medium',
    timeZone,
  })
  const formatDate = (start: string) => time.format(new Date(start))
  const formatValue = (value: number | null) =>
    value === null ? 'Не рассчитано' : number.format(value)
  const pointDetails = (name: string, start: string, value: number | null) =>
    `${name}, ${formatDate(start)}, ${formatValue(value)}${value === null ? '' : ` ${unit}`}`
  const hasPoints = visible.some(({ points }) => points.length > 0)
  const hasValues = visible.some(({ points }) =>
    points.some(({ value }) => value !== null),
  )
  const selectPoint = ({
    seriesId,
    metric_id,
    point_start,
  }: Omit<PlotPoint, 'details'>) => {
    setSelected({ id: seriesId, start: point_start })
    onPointSelect({ metric_id, point_start })
  }
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
      renderer: ({ hovered }) => (
        <ul className="cp-series-chart__tooltip">
          {hovered.map(
            ({ data }) =>
              data.custom && (
                <li key={data.custom.seriesId}>{data.custom.details}</li>
              ),
          )}
        </ul>
      ),
    },
    series: {
      data: visible.map(
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
            .map(({ point_start, value }) => {
              const details = pointDetails(name, point_start, value)
              return {
                x: Date.parse(point_start),
                y: value,
                custom: { seriesId: id, metric_id, point_start, details },
                ...(selected?.id === id && selected.start === point_start
                  ? { annotation: { label: { text: details } } }
                  : {}),
              }
            })
            .sort((a, b) => a.x - b.x),
        }),
      ),
    },
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
                {visible.flatMap(({ id, metric_id, name, points }) =>
                  points.map(({ point_start, value }) => {
                    const date = formatDate(point_start)
                    const display = formatValue(value)
                    const details = pointDetails(name, point_start, value)
                    const active =
                      selected?.id === id && selected.start === point_start
                    return (
                      <tr key={JSON.stringify([id, point_start])}>
                        <td>{name}</td>
                        <td>
                          <button
                            type="button"
                            aria-label={details}
                            aria-pressed={active}
                            onClick={() => {
                              selectPoint({
                                seriesId: id,
                                metric_id,
                                point_start,
                              })
                            }}
                          >
                            <time dateTime={point_start}>{date}</time>
                          </button>
                          <span
                            className="cp-series-chart__details"
                            aria-hidden="true"
                          >
                            {details}
                          </span>
                        </td>
                        <td>{display}</td>
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
