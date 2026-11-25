import { fireEvent, render, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { ChartProps, LineSeries } from '@gravity-ui/charts'
import { SeriesChart } from './series-chart'
import type { ChartSeries } from './series-chart'

const { chart } = vi.hoisted(() => ({
  chart: vi.fn<(props: ChartProps) => null>(() => null),
}))
vi.mock('@gravity-ui/charts', () => ({ Chart: chart }))

const plotted = () =>
  chart.mock.lastCall?.[0].data.series.data.filter(
    (series): series is LineSeries => series.type === 'line',
  )

const point = { point_start: '2026-10-01T00:00:00Z', value: 42 }
const series: ChartSeries[] = [
  { id: 'one', metric_id: 'T08', name: 'Время', points: [point] },
]

describe('SeriesChart', () => {
  it('renders the same points in the plot input and the accessible table', () => {
    render(
      <SeriesChart
        title="Динамика"
        summary="Значение за октябрь"
        unit="ч"
        timeZone="UTC"
        series={series}
        onPointSelect={vi.fn()}
      />,
    )
    expect(plotted()?.[0]?.data).toEqual([
      expect.objectContaining({
        x: Date.parse(point.point_start),
        y: point.value,
      }),
    ])
    expect(
      within(screen.getByRole('table', { name: 'Точки графика' })).getByRole(
        'cell',
        { name: '42' },
      ),
    ).toBeInTheDocument()
    expect(screen.getByText(/Часовой пояс: UTC/)).toBeInTheDocument()
  })

  it('formats point details in the requested time zone', () => {
    render(
      <SeriesChart
        title="Динамика"
        summary="Значение за октябрь"
        unit="ч"
        timeZone="Europe/Moscow"
        series={series}
        onPointSelect={vi.fn()}
      />,
    )
    const timestamp = Date.parse(point.point_start)
    expect(chart.mock.lastCall?.[0].data.xAxis?.title?.text).toBe(
      'Начало периода (UTC)',
    )
    expect(screen.getByText(/Часовой пояс: Europe\/Moscow/)).toBeVisible()
    const plottedPoint = plotted()?.[0]?.data[0]
    expect(plottedPoint?.x).toBe(timestamp)
    const custom: unknown = plottedPoint?.custom
    expect(custom).toHaveProperty(
      'details',
      expect.stringContaining('03:00:00'),
    )
    const button = screen.getByRole('button', { name: /03:00:00/ })
    expect(button).toHaveTextContent('03:00:00')
  })

  it('shows the sole remaining series after refreshing data', () => {
    const p50 = { id: 'p50', metric_id: 'T08', name: 'p50', points: [point] }
    const p85 = { ...p50, id: 'p85', name: 'p85' }
    const props = {
      title: 'Динамика',
      summary: 'Значение за октябрь',
      unit: 'ч',
      timeZone: 'UTC',
      onPointSelect: vi.fn(),
    }
    const { rerender } = render(<SeriesChart {...props} series={[p50, p85]} />)

    fireEvent.click(screen.getByRole('checkbox', { name: 'p50' }))
    expect(plotted()?.map(({ name }) => name)).toEqual(['p85'])

    rerender(<SeriesChart {...props} series={[p50]} />)
    expect(screen.queryByRole('checkbox')).toBeNull()
    expect(plotted()?.map(({ name }) => name)).toEqual(['p50'])
    const table = screen.getByRole('table', { name: 'Точки графика' })
    expect(within(table).getByRole('cell', { name: 'p50' })).toBeVisible()
    expect(within(table).getByRole('cell', { name: '42' })).toBeVisible()
  })

  it('limits the plot and table to five selected series', () => {
    const all = Array.from({ length: 6 }, (_, index) => ({
      id: String(index),
      metric_id: 'T08',
      name: `Серия ${String(index + 1)}`,
      points: [point],
    }))
    render(
      <SeriesChart
        title="Динамика"
        summary="Значение за октябрь"
        unit="ч"
        timeZone="UTC"
        series={all}
        onPointSelect={vi.fn()}
      />,
    )
    const table = screen.getByRole('table', { name: 'Точки графика' })
    const sixth = screen.getByRole('checkbox', { name: 'Серия 6' })
    expect(sixth).toBeDisabled()
    expect(plotted()?.map(({ name }) => name)).toEqual(
      all.slice(0, 5).map(({ name }) => name),
    )
    fireEvent.click(screen.getByRole('checkbox', { name: 'Серия 1' }))
    fireEvent.click(sixth)
    expect(plotted()?.map(({ name }) => name)).toEqual(
      all.slice(1).map(({ name }) => name),
    )
    expect(within(table).queryByRole('cell', { name: 'Серия 1' })).toBeNull()
    expect(
      within(table).getByRole('cell', { name: 'Серия 6' }),
    ).toBeInTheDocument()
  })

  it('exposes point details and selection through a focusable native button', () => {
    const onPointSelect = vi.fn()
    render(
      <SeriesChart
        title="Динамика"
        summary="Значение за октябрь"
        unit="ч"
        timeZone="UTC"
        series={series}
        onPointSelect={onPointSelect}
      />,
    )
    const button = screen.getByRole('button', { name: /Время, .*42 ч/ })
    button.focus()
    expect(button).toHaveFocus()
    // JSDOM does not synthesize the native Enter/Space click on a button.
    fireEvent.click(button, { detail: 0 })
    expect(onPointSelect).toHaveBeenCalledWith({
      metric_id: 'T08',
      point_start: point.point_start,
    })
    expect(button).toHaveAttribute('aria-pressed', 'true')
  })
})
