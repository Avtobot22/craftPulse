import { Button } from '@gravity-ui/uikit'
import { useState } from 'react'
import type { SubmitEvent } from 'react'
import type { TargetGroup, TargetPeriod } from '../api/target-api'
import { toTargetPeriod } from '../model/target-settings'

function textField(data: FormData, field: string) {
  const value = data.get(field)
  return typeof value === 'string' ? value : ''
}

export function TargetPeriodForm({
  from,
  to,
  loading,
  apply,
}: {
  from: string
  to: string
  loading: boolean
  apply: (period: TargetPeriod) => void
}) {
  const [error, setError] = useState(false)

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    const data = new FormData(event.currentTarget)
    const period = toTargetPeriod(
      textField(data, 'from'),
      textField(data, 'to'),
    )
    setError(!period)
    if (period) apply(period)
  }

  return (
    <form onSubmit={submit} noValidate>
      <fieldset>
        <legend>Период исторического ориентира</legend>
        <div className="cp-targets__fields">
          <label>
            Начало периода
            <input
              type="date"
              name="from"
              defaultValue={from}
              aria-invalid={error}
              aria-describedby={error ? 'target-period-error' : undefined}
            />
          </label>
          <label>
            Конец периода (исключительно)
            <input
              type="date"
              name="to"
              defaultValue={to}
              aria-invalid={error}
              aria-describedby={error ? 'target-period-error' : undefined}
            />
          </label>
          <Button type="submit" size="xl" view="outlined" disabled={loading}>
            Показать ориентир
          </Button>
        </div>
        <p>Даты по UTC. В расчёт входят полные недели внутри периода.</p>
        {error && (
          <p role="alert" id="target-period-error">
            Проверьте период. Конец должен быть позже начала.
          </p>
        )}
      </fieldset>
    </form>
  )
}

export function TargetGroupForm({
  group,
  select,
}: {
  group: TargetGroup | null
  select: (group: TargetGroup) => void
}) {
  const [error, setError] = useState(false)

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    const data = new FormData(event.currentTarget)
    const task_type = textField(data, 'task_type').trim()
    const size_label = textField(data, 'size_label').trim()
    const group = readTargetGroup(task_type, size_label)
    setError(!group)
    if (group) select(group)
  }

  return (
    <form onSubmit={submit} noValidate>
      <fieldset>
        <legend>Выбор группы</legend>
        <div className="cp-targets__fields">
          <label>
            Тип задачи
            <input
              name="task_type"
              defaultValue={group?.task_type ?? ''}
              maxLength={200}
              aria-invalid={error}
              aria-describedby={error ? 'target-group-error' : undefined}
            />
          </label>
          <label>
            Размер задачи
            <input
              name="size_label"
              defaultValue={group?.size_label ?? ''}
              maxLength={200}
              aria-invalid={error}
              aria-describedby={error ? 'target-group-error' : undefined}
            />
          </label>
          <Button type="submit" size="xl" view="outlined">
            Открыть группу
          </Button>
        </div>
        {error && (
          <p role="alert" id="target-group-error">
            Выберите тип и размер. Для задач без размера срок не применяется.
          </p>
        )}
      </fieldset>
    </form>
  )
}

export function readTargetGroup(
  taskType: string | null,
  size: string | null,
): TargetGroup | null {
  return taskType?.trim() &&
    size?.trim() &&
    taskType.length <= 200 &&
    size.length <= 200
    ? { task_type: taskType, size_label: size }
    : null
}
