import { Button, Label } from '@gravity-ui/uikit'
import { useState } from 'react'
import { statusOptions, taskStatusLabel } from '../model/task-list-search'

interface TaskFiltersProps {
  status: string
  overTarget: string
  onApply: (status: string, overTarget: string) => void
}

const deadlineOptions = [
  ['true', 'Срок превышен'],
  ['false', 'Срок не превышен'],
] as const

export function TaskFilters({ status, overTarget, onApply }: TaskFiltersProps) {
  const [draftStatus, setDraftStatus] = useState(status)
  const [draftDeadline, setDraftDeadline] = useState(overTarget)
  const appliedDeadline = deadlineOptions.find(
    ([value]) => value === overTarget,
  )?.[1]
  return (
    <form
      aria-label="Фильтры задач"
      onSubmit={(event) => {
        event.preventDefault()
        onApply(draftStatus, draftDeadline)
      }}
    >
      <div className="cp-task-list__filters">
        <label>
          Смысл статуса
          <select
            value={draftStatus}
            onChange={(event) => {
              setDraftStatus(event.target.value)
            }}
          >
            <option value="">Все статусы</option>
            {statusOptions.map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </select>
        </label>
        <label>
          Превышение срока
          <select
            value={draftDeadline}
            onChange={(event) => {
              setDraftDeadline(event.target.value)
            }}
          >
            <option value="">Все задачи</option>
            {deadlineOptions.map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </select>
        </label>
        <Button size="xl" view="action" type="submit">
          Применить
        </Button>
        <Button
          size="xl"
          disabled={!status && !overTarget && !draftStatus && !draftDeadline}
          onClick={() => {
            setDraftStatus('')
            setDraftDeadline('')
            onApply('', '')
          }}
        >
          Сбросить
        </Button>
      </div>
      {(status || appliedDeadline) && (
        <div
          className="cp-task-list__applied"
          role="group"
          aria-label="Применённые фильтры"
        >
          {status && <Label size="m">{taskStatusLabel(status)}</Label>}
          {appliedDeadline && <Label size="m">{appliedDeadline}</Label>}
        </div>
      )}
    </form>
  )
}
