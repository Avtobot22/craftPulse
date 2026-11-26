import { Button } from '@gravity-ui/uikit'
import { useState } from 'react'
import { SourceLink } from '@/entities/activity'
import { formatValue, MetricValue } from '@/entities/metric'
import { EmptyState, StaleState, StatusTime, TaskDetailLink } from '@/shared/ui'
import type {
  Catalog,
  MappingPreview as Preview,
} from '../api/workflow-responses'
import { UnmappedStatuses } from './unmapped-statuses'

const outcomes: Readonly<Record<string, string>> = {
  open: 'Открыта',
  completed: 'Завершена',
  cancelled: 'Отменена',
  duplicate: 'Дубликат',
  unknown: 'Неизвестно',
}

const controls = {
  T05: 'Завершено за неделю',
  T08: 'Медиана цикла за неделю (p50)',
  T12: 'В работе на дату среза',
}

export function MappingPreview({
  result,
  stale,
  catalog,
  teamId,
}: {
  result: Preview
  stale: boolean
  catalog: Catalog
  teamId: string
}) {
  const [all, setAll] = useState(false)
  const canExpand = result.tasks.length > 10
  const sample = all ? result.tasks : result.tasks.slice(0, 10)
  return (
    <section aria-labelledby="workflow-preview-result">
      <h3 id="workflow-preview-result" tabIndex={-1}>
        Результат предпросмотра
      </h3>
      {stale && (
        <StaleState snapshotAt={result.as_of}>
          <p>Правила изменились. Запустите предпросмотр снова.</p>
        </StaleState>
      )}
      <p>
        Дата среза <StatusTime value={result.as_of} /> · Задач в выборке:{' '}
        {result.tasks.length}
        {canExpand && !all && ` · Показано ${String(sample.length)}`}
      </p>
      {!sample.length ? (
        <EmptyState
          reason="history"
          description="Нет задач для проверки. Проверьте состояние сбора истории в подключениях."
        />
      ) : (
        <div
          className="cp-workflow__table"
          role="region"
          aria-label="Примеры задач"
          tabIndex={0}
        >
          <table aria-label="Задачи предпросмотра">
            <thead>
              <tr>
                {[
                  'Задача',
                  'Начало',
                  'Завершение',
                  'Исход',
                  'Блокировка',
                  'Тип',
                  'Размер',
                  'Без правила',
                ].map((label) => (
                  <th key={label} scope="col">
                    {label}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {sample.map(
                ({
                  task,
                  started_at,
                  completed_at,
                  outcome,
                  blocked_seconds,
                  task_type,
                  size_label,
                  unmapped,
                }) => {
                  const name = task.display_key ?? task.title ?? task.id
                  return (
                    <tr key={task.id}>
                      <th scope="row">
                        {task.source_url ? (
                          <SourceLink url={task.source_url}>{name}</SourceLink>
                        ) : (
                          <TaskDetailLink
                            taskId={task.id}
                            params={{ team_id: teamId }}
                          >
                            {name}
                          </TaskDetailLink>
                        )}
                        {task.display_key && task.title && (
                          <small>{task.title}</small>
                        )}
                      </th>
                      <td>
                        {started_at ? (
                          <StatusTime value={started_at} />
                        ) : (
                          'Нет начала'
                        )}
                      </td>
                      <td>
                        {completed_at ? (
                          <StatusTime value={completed_at} />
                        ) : (
                          'Нет завершения'
                        )}
                      </td>
                      <td>
                        {Object.hasOwn(outcomes, outcome)
                          ? outcomes[outcome]
                          : outcomes.unknown}
                      </td>
                      <td>
                        {blocked_seconds === null
                          ? 'Нет данных'
                          : formatValue(blocked_seconds, 'seconds')}
                      </td>
                      <td>{task_type ?? 'Неизвестен'}</td>
                      <td>{size_label ?? 'Без размера'}</td>
                      <td>{unmapped.length ? unmapped.join(', ') : '—'}</td>
                    </tr>
                  )
                },
              )}
            </tbody>
          </table>
        </div>
      )}
      {canExpand && (
        <Button
          type="button"
          view="outlined"
          aria-expanded={all}
          onClick={() => {
            setAll(!all)
          }}
        >
          {all ? 'Показать первые 10 задач' : 'Показать оставшиеся задачи'}
        </Button>
      )}
      <UnmappedStatuses statuses={result.unmapped_statuses} catalog={catalog} />
      {!!sample.length && (
        <>
          <h3>Контрольные показатели</h3>
          <p>
            Неделя отчёта с <StatusTime value={result.period.from} /> по{' '}
            <StatusTime value={result.period.to} />. Сверьте результаты с
            задачами в трекере.
          </p>
          <dl className="cp-workflow__metrics">
            {Object.entries(controls).map(([id, name]) => {
              const control = result.control_values.find(
                (value) => value.metric_id === id,
              )
              return (
                <div key={id}>
                  <dt>{name}</dt>
                  <dd>
                    {control ? (
                      <MetricValue value={control.value} />
                    ) : (
                      'Нет данных'
                    )}
                  </dd>
                </div>
              )
            })}
          </dl>
        </>
      )}
    </section>
  )
}
