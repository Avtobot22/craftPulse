import { Button, Label, SegmentedRadioGroup } from '@gravity-ui/uikit'
import { useId, useState } from 'react'
import type { SubmitEvent } from 'react'
import type { TeamReportScope } from '@/shared/config'
import { isDateTime } from '@/shared/api'
import { periodError } from '@/shared/config'
import type { ReportParams } from '@/shared/config'

type Project = TeamReportScope['projects'][number]

function localTime(value: string) {
  return isDateTime(value) ? new Date(value).toISOString().slice(0, -1) : ''
}

function fromLocalTime(value: string, previous: string) {
  if (value === localTime(previous)) return previous
  const instant = new Date(value + 'Z')
  return Number.isFinite(instant.getTime()) ? instant.toISOString() : ''
}

const groupLabels = {
  task_type: 'По типу задачи',
  size_label: 'По размеру задачи',
}

export function ReportControls({
  applied,
  projects,
  scopeProject,
  onApply,
  onReset,
  onStep,
}: {
  applied: ReportParams
  projects: readonly Project[]
  scopeProject: Project | undefined
  onApply: (params: ReportParams) => void
  onReset: () => void
  onStep: (step: ReportParams['step']) => void
}) {
  const formId = useId()
  const [expanded, setExpanded] = useState(false)
  const appliedKey = JSON.stringify([
    applied.from,
    applied.to,
    applied.projects,
    applied.taskType,
    applied.size,
    applied.groupBy,
  ])
  const [edit, setEdit] = useState<{ key: string; draft: ReportParams }>()
  const draft = edit?.key === appliedKey ? edit.draft : applied
  const setDraft = (next: ReportParams) => {
    setEdit({ key: appliedKey, draft: next })
  }
  const [error, setError] = useState<string>()
  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    const next = {
      ...draft,
      taskType: draft.taskType.trim(),
      size: draft.size.trim(),
    }
    const validation = periodError(next)
    setError(validation)
    if (!validation) {
      setEdit(undefined)
      onApply(next)
    }
  }

  const appliedProjects = scopeProject
    ? [scopeProject.name]
    : applied.projects.map(
        (id) =>
          projects.find((project) => project.id === id)?.name ??
          'Недоступный проект',
      )
  const labels = [
    ...appliedProjects.map((name) => `Проект: ${name}`),
    ...(applied.taskType ? [`Тип: ${applied.taskType}`] : []),
    ...(applied.size
      ? [`Размер: ${applied.size === 'unknown' ? 'Без размера' : applied.size}`]
      : []),
    ...(applied.groupBy ? [groupLabels[applied.groupBy]] : []),
  ]

  return (
    <section className="cp-team-report__panel" aria-label="Период и фильтры">
      <fieldset className="cp-team-report__steps">
        <legend>Масштаб</legend>
        <SegmentedRadioGroup<ReportParams['step']>
          name="report-step"
          size="xl"
          value={applied.step}
          onUpdate={onStep}
          options={[
            { value: 'week', content: 'Неделя' },
            { value: 'month', content: 'Месяц' },
          ]}
        />
      </fieldset>
      <div className="cp-team-report__filter-toggle">
        <Button
          size="xl"
          aria-controls={formId}
          aria-expanded={expanded}
          onClick={() => {
            setExpanded(!expanded)
          }}
        >
          {expanded ? 'Скрыть фильтры' : 'Показать фильтры'}
          {labels.length > 0 && ` (${String(labels.length)})`}
        </Button>
      </div>
      <form
        className="cp-team-report__filters"
        id={formId}
        data-expanded={expanded}
        onSubmit={submit}
      >
        <label>
          Начало периода (UTC, включительно)
          <input
            type="datetime-local"
            step="any"
            required
            value={localTime(draft.from)}
            aria-invalid={Boolean(error)}
            onChange={(event) => {
              setDraft({
                ...draft,
                from: fromLocalTime(event.currentTarget.value, applied.from),
              })
            }}
          />
        </label>
        <label>
          Конец периода (UTC, исключительно)
          <input
            type="datetime-local"
            step="any"
            required
            value={localTime(draft.to)}
            aria-invalid={Boolean(error)}
            onChange={(event) => {
              setDraft({
                ...draft,
                to: fromLocalTime(event.currentTarget.value, applied.to),
              })
            }}
          />
        </label>
        {!scopeProject && (
          <label>
            Проекты отчёта
            <select
              multiple
              size={3}
              value={draft.projects}
              onChange={(event) => {
                setDraft({
                  ...draft,
                  projects: Array.from(
                    event.currentTarget.selectedOptions,
                    ({ value }) => value,
                  ).slice(0, 50),
                })
              }}
            >
              {projects.map(({ id, name }) => (
                <option value={id} key={id}>
                  {name}
                </option>
              ))}
            </select>
          </label>
        )}
        <label>
          Тип задачи
          <input
            maxLength={200}
            value={draft.taskType}
            onChange={(event) => {
              setDraft({
                ...draft,
                taskType: event.currentTarget.value,
              })
            }}
          />
        </label>
        <label>
          Размер задачи
          <input
            list="team-report-sizes"
            maxLength={200}
            value={draft.size}
            onChange={(event) => {
              setDraft({ ...draft, size: event.currentTarget.value })
            }}
          />
        </label>
        <datalist id="team-report-sizes">
          <option value="unknown">Без размера</option>
        </datalist>
        <label>
          Разбиение
          <select
            value={draft.groupBy}
            onChange={(event) => {
              const value = event.currentTarget.value
              setDraft({
                ...draft,
                groupBy:
                  value === 'task_type' || value === 'size_label' ? value : '',
              })
            }}
          >
            <option value="">Без разбиения</option>
            <option value="task_type">{groupLabels.task_type}</option>
            <option value="size_label">{groupLabels.size_label}</option>
          </select>
        </label>
        {error && <p role="alert">{error}</p>}
        <div className="cp-team-report__actions">
          <Button type="submit" view="action" size="xl">
            Применить
          </Button>
          <Button
            size="xl"
            onClick={() => {
              setEdit(undefined)
              setError(undefined)
              onReset()
            }}
          >
            Сбросить
          </Button>
        </div>
      </form>
      {labels.length > 0 && (
        <ul className="cp-team-report__chips" aria-label="Применённые фильтры">
          {labels.map((label, index) => (
            <li key={`${label}-${String(index)}`}>
              <Label>{label}</Label>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
