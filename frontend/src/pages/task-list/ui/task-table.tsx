import { Button, Label } from '@gravity-ui/uikit'
import type { MouseEvent } from 'react'
import { SourceLink } from '@/entities/activity'
import { formatValue } from '@/entities/metric'
import type { components, OpenCodes } from '@/shared/api'
import type { TaskDetailSearch } from '@/shared/config'
import { StatusTime, TaskDetailLink } from '@/shared/ui'
import { taskStatusLabel } from '../model/task-list-search'
import type { Sort } from '../model/task-list-search'

type Task = OpenCodes<components['schemas']['TaskSummary']>
type SortField = Exclude<Sort, `-${string}`>

function SortHeader({
  label,
  field,
  sort,
  onSort,
  initial,
}: {
  label: string
  field: SortField
  sort: Sort
  initial: Sort
  onSort: (sort: Sort) => void
}) {
  const descending = `-${field}` as const
  const direction =
    sort === field
      ? 'ascending'
      : sort === descending
        ? 'descending'
        : undefined
  const next =
    direction === 'ascending' ? descending : direction ? field : initial
  return (
    <th scope="col" aria-sort={direction}>
      <Button
        size="xl"
        view="flat"
        onClick={() => {
          onSort(next)
        }}
      >
        {label}
        {direction && (
          <span className="cp-task-list__sort">
            {direction === 'ascending' ? 'По возрастанию' : 'По убыванию'}
          </span>
        )}
      </Button>
    </th>
  )
}

function Deadline({ task }: { task: Task }) {
  if (task.over_target === true)
    return (
      <Label theme="warning" size="m">
        Срок превышен
      </Label>
    )
  if (task.over_target === false) return 'В сроке'
  return task.target_configured ? 'Превышение неизвестно' : 'Срок не задан'
}

function openTaskFromRow(event: MouseEvent<HTMLTableRowElement>) {
  if (
    !(event.target instanceof Element) ||
    event.target.closest('a') ||
    event.defaultPrevented ||
    event.ctrlKey ||
    event.metaKey ||
    event.shiftKey ||
    event.altKey ||
    event.currentTarget.ownerDocument.getSelection()?.isCollapsed === false
  )
    return
  event.currentTarget.querySelector<HTMLAnchorElement>('th a')?.click()
}

export function TaskTable({
  tasks,
  scope,
  sort,
  onSort,
}: {
  tasks: readonly Task[]
  scope: TaskDetailSearch
  sort: Sort
  onSort: (sort: Sort) => void
}) {
  return (
    <div
      className="cp-task-list__scroll"
      role="region"
      aria-label="Таблица задач"
      tabIndex={0}
    >
      <table className="cp-task-list__table" aria-label="Задачи области">
        <thead>
          <tr>
            <SortHeader
              label="Ключ и название"
              field="key"
              sort={sort}
              initial="key"
              onSort={onSort}
            />
            <th scope="col">Статус</th>
            <th scope="col">Тип</th>
            <th scope="col">Размер</th>
            <SortHeader
              label="Возраст эпизода"
              field="age"
              sort={sort}
              initial="-age"
              onSort={onSort}
            />
            <th scope="col">Целевой срок</th>
            <SortHeader
              label="Обновлено"
              field="updated_at"
              sort={sort}
              initial="-updated_at"
              onSort={onSort}
            />
            <th scope="col">Источник</th>
          </tr>
        </thead>
        <tbody>
          {tasks.map((task) => {
            const name =
              [task.key, task.title].filter(Boolean).join(' · ') ||
              'Открыть задачу'
            return (
              <tr key={task.id} onClick={openTaskFromRow}>
                <th scope="row">
                  <TaskDetailLink
                    taskId={task.id}
                    params={scope}
                    preserveHistoryFocus
                  >
                    {name}
                  </TaskDetailLink>
                </th>
                <td>
                  {taskStatusLabel(task.status.meaning)}
                  {task.status.name && (
                    <span className="cp-task-list__native-status">
                      {task.status.name}
                    </span>
                  )}
                </td>
                <td>{task.task_type ?? 'Тип неизвестен'}</td>
                <td>
                  {task.size_label?.trim() ? task.size_label : 'Без размера'}
                </td>
                <td>
                  {task.age_seconds === null
                    ? 'Начало неизвестно'
                    : formatValue(task.age_seconds, 'seconds')}
                </td>
                <td>
                  <Deadline task={task} />
                </td>
                <td>
                  <StatusTime value={task.updated_at} />
                </td>
                <td>
                  {task.source_url ? (
                    <SourceLink url={task.source_url} sourceName="Jira" />
                  ) : (
                    'Нет ссылки'
                  )}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
