import { formatValue } from '@/entities/metric'
import type { CohortItem, TeamReport } from '@/shared/api'
import { ReportTaskLink } from './report-task-link'

const signalLabels: Readonly<Record<string, string>> = {
  awaiting_reviewer: 'Нет ревьюера',
  awaiting_first_response: 'Нет первого ответа человека',
  ci_failed: 'Ошибка или тайм-аут CI',
  ci_passed_after_retry: 'Fail → pass',
  ci_running: 'CI выполняется',
  missing_report: 'Отсутствует ожидаемый отчёт',
}

function duration(value: number | null) {
  return value === null ? 'Неизвестно' : formatValue(value, 'seconds')
}

export function CrossTaskTable({
  tasks,
  report,
  point,
  captionId,
  focusPrefix,
  canReadTasks,
}: {
  tasks: Extract<CohortItem, { deleted: false }>[]
  report: TeamReport
  point: string
  captionId: string
  focusPrefix: string
  canReadTasks: boolean
}) {
  return (
    <div className="cp-team-report__table-scroll">
      <table aria-labelledby={captionId}>
        <thead>
          <tr>
            <th scope="col">Задача</th>
            <th scope="col">Возраст задачи</th>
            <th scope="col">Ожидание MR</th>
            <th scope="col">Признаки</th>
          </tr>
        </thead>
        <tbody>
          {tasks.map(({ object, itemNo, taskAge, value, signals }) => {
            const label =
              [object.display_key, object.title].filter(Boolean).join(' · ') ||
              'Задача'
            return (
              <tr key={itemNo}>
                <th scope="row">
                  {canReadTasks ? (
                    <ReportTaskLink
                      report={report}
                      taskId={object.id}
                      point={point}
                      pointParam="cross_point"
                      focusId={`${focusPrefix}-task-${object.id}`}
                    >
                      {label}
                    </ReportTaskLink>
                  ) : (
                    label
                  )}
                </th>
                <td>{duration(taskAge)}</td>
                <td>{duration(value)}</td>
                <td>
                  {signals
                    .map((signal) =>
                      Object.hasOwn(signalLabels, signal)
                        ? signalLabels[signal]
                        : signal,
                    )
                    .join(', ') || 'Нет данных'}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
