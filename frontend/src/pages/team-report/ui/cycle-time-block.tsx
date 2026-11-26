import { Card } from '@gravity-ui/uikit'
import { useState } from 'react'
import { SourceLink } from '@/entities/activity'
import { formatValue, metricReason } from '@/entities/metric'
import type { ApiClient } from '@/shared/api'
import type { TaskContribution } from '../api/metric-details'
import {
  groupLabel,
  reportSteps,
  useTaskContributions,
} from '../model/metric-details'
import type { ReportBlockProps } from '../model/metrics'
import { DetailPoint, DetailsStatus, StepSelect } from './detail-point'

type Duration = TaskContribution & { value: number }
function isDuration(row: TaskContribution): row is Duration {
  return row.role === 'numerator' && row.value !== null
}

function groupDurations(tasks: Duration[]) {
  const sizes = new Map<string | null, Duration[]>()
  const groups = new Map<string, Duration[]>()
  for (const task of tasks) {
    const size = sizes.get(task.size) ?? []
    size.push(task)
    sizes.set(task.size, size)
    const key = JSON.stringify([task.taskType, task.size])
    const group = groups.get(key) ?? []
    group.push(task)
    groups.set(key, group)
  }
  return { sizes, groups }
}

function sortSizes(a: string | null, b: string | null) {
  if (a === b) return 0
  if (a === null) return 1
  if (b === null) return -1
  return a.localeCompare(b, 'ru', { numeric: true })
}

function ScatterColumn({
  block,
  step,
  size,
  sample,
  max,
  series,
}: {
  block: ReportBlockProps
  step: string
  size: string | null
  sample: Duration[]
  max: number
  series: ReportBlockProps['report']['metrics'][number] | undefined
}) {
  const lanes: number[] = []
  const points = [...sample]
    .sort((a, b) => a.value - b.value)
    .map((task) => {
      const height = max ? (task.value / max) * 100 : 0
      const available = lanes.findIndex(
        (lastHeight) => height - lastHeight >= 14,
      )
      const lane = available === -1 ? lanes.length : available
      lanes[lane] = height
      return { task, height, lane }
    })
  return (
    <div
      className="cp-team-report__scatter-column"
      style={{ minWidth: Math.max(110, lanes.length * 28) }}
    >
      <div className="cp-team-report__scatter-points">
        {points.map(({ task, height, lane }) => (
          <span
            key={task.id}
            className="cp-team-report__scatter-point"
            style={{
              bottom: `${String(height)}%`,
              left: `${String(((lane + 0.5) / lanes.length) * 100)}%`,
            }}
            title={`${task.label} · ${String(task.value)} с`}
          >
            <SourceLink url={task.sourceUrl}>
              <span className="cp-team-report__sr-only">
                {task.label} · {task.value} с
              </span>
            </SourceLink>
          </span>
        ))}
      </div>
      <div>
        {size ?? 'Без размера'} ·{' '}
        <DetailPoint
          block={block}
          series={series}
          step={step}
          id={`cycle-size-${JSON.stringify(size)}`}
          cohort={{ taskType: block.taskType, size }}
        >
          n = {sample.length}
        </DetailPoint>
      </div>
    </div>
  )
}

function Distribution({
  block,
  step,
  tasks,
}: {
  block: ReportBlockProps
  step: string
  tasks: Duration[]
}) {
  const { sizes, groups } = groupDurations(tasks)
  const series = block.report.metrics.filter(
    (series) =>
      series.metric_id === 'T08' &&
      series.points.some(({ point_start }) => point_start === step),
  )
  const p50 = series.find(({ estimator }) => estimator === 'p50') ?? series[0]
  const p85 = series.find(({ estimator }) => estimator === 'p85')
  const incomplete = series.some(({ points }) =>
    points.some(
      ({ point_start, value }) =>
        point_start === step &&
        (value.state === 'PROVISIONAL' || value.missing_count > 0),
    ),
  )
  const expected = series
    .filter(({ estimator }) => estimator === 'p50' || estimator === undefined)
    .map(
      ({ points }) =>
        points.find(({ point_start }) => point_start === step)?.value.n,
    )
  const missingPoints =
    expected.every((n) => typeof n === 'number') &&
    expected.reduce((total, n) => total + n, 0) > tasks.length
  const max = tasks.reduce((top, row) => Math.max(top, row.value), 0)
  return (
    <>
      {(incomplete || missingPoints) && (
        <p>
          Частичные данные. Распределение построено по доступным длительностям.
        </p>
      )}
      <div className="cp-team-report__table-scroll">
        <table>
          <caption>Длительность по типу и размеру</caption>
          <thead>
            <tr>
              <th scope="col">Тип и размер</th>
              <th scope="col">p50</th>
              <th scope="col">p85</th>
              <th scope="col">Задач (n)</th>
            </tr>
          </thead>
          <tbody>
            {[...groups].map(([key, group]) => {
              const first = group[0]
              if (!first) return null
              const { taskType, size } = first
              const cohort = { taskType, size }
              const sorted = group
                .map(({ value }) => value)
                .sort((a, b) => a - b)
              const median =
                sorted[Math.ceil(sorted.length * 0.5) - 1] ?? first.value
              const upper =
                sorted[Math.ceil(sorted.length * 0.85) - 1] ?? first.value
              return (
                <tr key={key}>
                  <th scope="row">{groupLabel(taskType, size)}</th>
                  <td>
                    <DetailPoint
                      block={block}
                      series={p50}
                      step={step}
                      id={`cycle-p50-${key}`}
                      cohort={cohort}
                    >
                      {formatValue(median, 'seconds')}
                    </DetailPoint>
                  </td>
                  <td>
                    <DetailPoint
                      block={block}
                      series={p85 ?? p50}
                      step={step}
                      id={`cycle-p85-${key}`}
                      cohort={cohort}
                    >
                      {formatValue(upper, 'seconds')}
                    </DetailPoint>
                    {group.length < 20 && <span> · Малая выборка</span>}
                  </td>
                  <td>
                    <DetailPoint
                      block={block}
                      series={p50}
                      step={step}
                      id={`cycle-n-${key}`}
                      cohort={cohort}
                    >
                      {group.length}
                    </DetailPoint>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
      <figure
        className="cp-team-report__duration"
        aria-labelledby="team-report-scatter-caption"
      >
        <figcaption id="team-report-scatter-caption">
          Длительность отдельных задач. Точка ведёт к задаче в источнике.
        </figcaption>
        <div className="cp-team-report__scatter-scroll">
          <div className="cp-team-report__scatter-scale" aria-hidden="true">
            <span>{formatValue(max, 'seconds')}</span>
            <span>0 ч</span>
          </div>
          {[...sizes]
            .sort(([a], [b]) => sortSizes(a, b))
            .map(([size, sample]) => (
              <ScatterColumn
                key={JSON.stringify(size)}
                block={block}
                step={step}
                size={size}
                sample={sample}
                max={max}
                series={p50}
              />
            ))}
        </div>
      </figure>
      <details>
        <summary>Таблица точек диаграммы</summary>
        <div className="cp-team-report__table-scroll">
          <table>
            <thead>
              <tr>
                <th scope="col">Задача</th>
                <th scope="col">Тип и размер</th>
                <th scope="col">Длительность, с</th>
              </tr>
            </thead>
            <tbody>
              {tasks.map((task) => (
                <tr key={task.id}>
                  <th scope="row">
                    <SourceLink url={task.sourceUrl}>{task.label}</SourceLink>
                  </th>
                  <td>{groupLabel(task.taskType, task.size)}</td>
                  <td>{task.value}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </details>
    </>
  )
}

export function CycleTimeBlock({
  block,
  api,
}: {
  block: ReportBlockProps
  api: ApiClient
}) {
  const steps = reportSteps(block, ['T08'])
  const [selected, setSelected] = useState(
    block.focus?.seriesId.startsWith('["T08"') ? block.focus.pointStart : '',
  )
  const step = (steps.includes(selected) ? selected : steps[0]) ?? ''
  const query = useTaskContributions(api, block, 'T08', step)
  if (!step) return null
  const tasks = query.isError ? [] : (query.data?.filter(isDuration) ?? [])
  const first = block.report.metrics.find(
    ({ metric_id, points }) =>
      metric_id === 'T08' &&
      points.some(({ point_start }) => point_start === step),
  )
  const point = first?.points.find(({ point_start }) => point_start === step)
  return (
    <section
      className="cp-team-report__block"
      aria-labelledby="team-report-cycle"
    >
      <h2 id="team-report-cycle">Длительность цикла</h2>
      <Card
        className="cp-team-report__panel cp-team-report__detail"
        view="outlined"
      >
        <StepSelect
          steps={steps}
          step={step}
          label="Шаг длительности"
          timeZone={block.metadata?.timeZone ?? 'UTC'}
          onChange={setSelected}
        />
        <DetailsStatus
          query={query}
          reason="Не удалось прочитать когорту длительностей"
        />
        {tasks.length > 0 ? (
          <Distribution block={block} step={step} tasks={tasks} />
        ) : (
          !query.isFetching &&
          !query.isError && (
            <p>
              <DetailPoint
                block={block}
                series={first}
                step={step}
                id="cycle-unavailable"
              >
                Длительность не рассчитана
              </DetailPoint>
              {' · '}
              {metricReason(point?.value.reason_code ?? null) ??
                'Нет доступных вкладов завершённых задач'}
            </p>
          )
        )}
      </Card>
    </section>
  )
}
