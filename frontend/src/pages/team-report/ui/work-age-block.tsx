import { Card } from '@gravity-ui/uikit'
import { useState } from 'react'
import { useSearchParams } from 'react-router'
import { SourceLink } from '@/entities/activity'
import { formatValue, metricReason } from '@/entities/metric'
import type { ApiClient } from '@/shared/api'
import type { TaskContribution } from '../api/metric-details'
import {
  reportSteps,
  taskSeries,
  useTaskContributions,
} from '../model/metric-details'
import type { ReportBlockProps } from '../model/metrics'
import { DetailPoint, DetailsStatus, StepSelect } from './detail-point'
import { OutcomePoints } from './outcome-metrics-block'
import { ReportTaskLink } from './report-task-link'

interface WorkTask {
  task: TaskContribution
  episode?: TaskContribution
  full?: TaskContribution
  target?: TaskContribution
}

function collectWork(ages: TaskContribution[], targets: TaskContribution[]) {
  const tasks = new Map<string, WorkTask>()
  for (const row of ages) {
    if (row.role !== 'numerator') continue
    const entry = tasks.get(row.id) ?? { task: row }
    if (row.reason === 'full_age') entry.full = row
    else entry.episode = row
    tasks.set(row.id, entry)
  }
  for (const row of targets) {
    const entry = tasks.get(row.id)
    if (entry && row.role !== 'numerator') entry.target = row
  }
  return [...tasks.values()].sort(
    (a, b) => (b.episode?.value ?? 0) - (a.episode?.value ?? 0),
  )
}

function displayDuration(row: TaskContribution | undefined) {
  return row?.value == null
    ? 'Не рассчитано'
    : formatValue(row.value, 'seconds')
}

function AgeTable({
  block,
  step,
  tasks,
  canReadTasks,
}: {
  block: ReportBlockProps
  step: string
  tasks: WorkTask[]
  canReadTasks: boolean
}) {
  return (
    <div className="cp-team-report__table-scroll">
      <table>
        <caption>Возраст задач в работе</caption>
        <thead>
          <tr>
            <th scope="col">Задача</th>
            <th scope="col">Текущий эпизод</th>
            <th scope="col">С первого начала работы</th>
            <th scope="col">Возраст / целевой срок</th>
          </tr>
        </thead>
        <tbody>
          {tasks.map(({ task, episode, full, target }) => {
            const ageSeries = taskSeries(block, 'T14', step, task)
            const targetSeries = taskSeries(block, 'T11', step, task)
            return (
              <tr key={task.id}>
                <th scope="row">
                  {canReadTasks ? (
                    <ReportTaskLink
                      report={block.report}
                      taskId={task.id}
                      point={step}
                      pointParam="work_age_point"
                      focusId={`work-age-task-${task.id}`}
                    >
                      {task.label}
                    </ReportTaskLink>
                  ) : (
                    task.label
                  )}
                  {canReadTasks && task.sourceUrl && (
                    <>
                      {' '}
                      ·{' '}
                      <SourceLink
                        url={task.sourceUrl}
                        sourceName="Jira Cloud"
                      />
                    </>
                  )}
                </th>
                <td>
                  <DetailPoint
                    block={block}
                    series={ageSeries}
                    step={step}
                    id={`episode-${task.id}`}
                  >
                    {displayDuration(episode)}
                  </DetailPoint>
                </td>
                <td>
                  <DetailPoint
                    block={block}
                    series={ageSeries}
                    step={step}
                    id={`full-${task.id}`}
                  >
                    {displayDuration(full)}
                  </DetailPoint>
                </td>
                <td>
                  <DetailPoint
                    block={block}
                    series={targetSeries}
                    step={step}
                    id={`target-ratio-${task.id}`}
                  >
                    {target?.value == null
                      ? [
                          'Не рассчитано',
                          metricReason(target?.reason ?? null) ??
                            'Нет данных о применимом сроке',
                        ].join(' · ')
                      : formatValue(target.value, 'ratio')}
                  </DetailPoint>
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

function BlockedTimeTable({
  block,
  step,
  rows,
}: {
  block: ReportBlockProps
  step: string
  rows: TaskContribution[]
}) {
  const elapsed = new Map(
    rows
      .filter(({ role }) => role === 'denominator')
      .map(({ id, value }) => [id, value]),
  )
  const blocked = rows.filter(
    ({ role, value }) => role === 'numerator' && value !== null && value > 0,
  )
  if (!blocked.length)
    return <p>В доступных данных за этот шаг блокировок нет.</p>
  return (
    <div className="cp-team-report__table-scroll">
      <table>
        <caption>Блокировки за шаг</caption>
        <thead>
          <tr>
            <th scope="col">Задача</th>
            <th scope="col">Время блокировки</th>
            <th scope="col">Доля времени в работе</th>
          </tr>
        </thead>
        <tbody>
          {blocked.map((task) => {
            const total = elapsed.get(task.id)
            const series = taskSeries(block, 'T15', step, task)
            const share =
              total && total > 0 && task.value !== null
                ? formatValue(task.value / total, 'ratio')
                : 'Не рассчитано'
            return (
              <tr key={task.id}>
                <th scope="row">{task.label}</th>
                <td>
                  <DetailPoint
                    block={block}
                    series={series}
                    step={step}
                    id={`blocked-time-${task.id}`}
                  >
                    {displayDuration(task)}
                  </DetailPoint>
                </td>
                <td>
                  <DetailPoint
                    block={block}
                    series={series}
                    step={step}
                    id={`blocked-share-${task.id}`}
                  >
                    {share}
                  </DetailPoint>
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

export function WorkAgeBlock({
  block,
  api,
  canReadTasks,
}: {
  block: ReportBlockProps
  api: ApiClient
  canReadTasks: boolean
}) {
  const steps = reportSteps(block, ['T14', 'T11', 'T15'])
  const [search] = useSearchParams()
  const focus = block.focus
  const [selected, setSelected] = useState(
    focus && /^\["T(14|11|15)"/u.test(focus.seriesId)
      ? focus.pointStart
      : (search.get('work_age_point') ?? ''),
  )
  const step = (steps.includes(selected) ? selected : steps[0]) ?? ''
  const age = useTaskContributions(api, block, 'T14', step)
  const target = useTaskContributions(api, block, 'T11', step)
  const blocked = useTaskContributions(api, block, 'T15', step)
  if (!step) return null
  const tasks = collectWork(
    age.isError ? [] : (age.data ?? []),
    target.isError ? [] : (target.data ?? []),
  )

  return (
    <section
      className="cp-team-report__block"
      aria-labelledby="team-report-work-age"
    >
      <h2 id="team-report-work-age">Возраст текущей работы и блокировки</h2>
      <Card
        className="cp-team-report__panel cp-team-report__detail"
        view="outlined"
      >
        <StepSelect
          steps={steps}
          step={step}
          label="Шаг возраста и блокировок"
          timeZone={block.metadata?.timeZone ?? 'UTC'}
          onChange={setSelected}
        />
        <DetailsStatus
          query={age}
          reason="Не удалось прочитать возраст задач"
        />
        <DetailsStatus
          query={target}
          reason="Не удалось прочитать отношение возраста к сроку"
        />
        {tasks.length > 0 && (
          <AgeTable
            block={block}
            step={step}
            tasks={tasks}
            canReadTasks={canReadTasks}
          />
        )}
        {!tasks.length && age.isSuccess && (
          <p>Нет задач с известным возрастом текущего эпизода.</p>
        )}
        {taskSeries(block, 'T11', step) && (
          <>
            <h3>Превышения целевых сроков</h3>
            <p>Для задач без целевого срока превышение не рассчитывается.</p>
            <OutcomePoints block={block} metricId="T11" step={step} />
          </>
        )}
        <DetailsStatus
          query={blocked}
          reason="Не удалось прочитать время блокировок"
        />
        {!blocked.isError && blocked.data && (
          <BlockedTimeTable block={block} step={step} rows={blocked.data} />
        )}
      </Card>
    </section>
  )
}
