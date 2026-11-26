import { formatTime } from '../model/metrics'

const groups = {
  task_type: 'по типу задачи',
  size_label: 'по размеру задачи',
}

export function SavedViewFilters({
  from,
  to,
  step,
  projects,
  taskType,
  size,
  groupBy,
}: {
  from: string
  to: string
  step: 'week' | 'month'
  projects: readonly string[]
  taskType?: string | undefined
  size?: string | undefined
  groupBy?: keyof typeof groups | undefined
}) {
  return (
    <dl className="cp-team-report__revision" aria-label="Параметры среза">
      <div>
        <dt>Период (UTC)</dt>
        <dd>
          <time dateTime={from}>{formatTime(from, 'UTC')}</time>
          {' - '}
          <time dateTime={to}>{formatTime(to, 'UTC')}</time>
        </dd>
      </div>
      <div>
        <dt>Шаг</dt>
        <dd>{step === 'week' ? 'Неделя' : 'Месяц'}</dd>
      </div>
      {projects.length > 0 && (
        <div>
          <dt>Проекты</dt>
          <dd>{projects.join(', ')}</dd>
        </div>
      )}
      {taskType && (
        <div>
          <dt>Тип задачи</dt>
          <dd>{taskType}</dd>
        </div>
      )}
      {size && (
        <div>
          <dt>Размер задачи</dt>
          <dd>{size === 'unknown' ? 'Без размера' : size}</dd>
        </div>
      )}
      {groupBy && (
        <div>
          <dt>Разбиение</dt>
          <dd>{groups[groupBy]}</dd>
        </div>
      )}
    </dl>
  )
}
