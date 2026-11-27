import { ApiError } from '@/shared/api'
import type { ChangeProposal, ChangeValidation } from '@/shared/ui'
import type {
  TargetDraft,
  TargetPeriod,
  TargetVersion,
} from '../api/target-api'

export interface HistoricalBasis {
  seconds: number
  revision: string
  period: TargetPeriod
  lowSample: boolean
}

export interface TargetInput {
  days: string
  hours: string
  historical: HistoricalBasis | null
}

export interface FormIssue {
  field: 'target-days' | 'target-basis' | null
  message: string
}

const hour = 3600
const day = 24 * hour

export function parseDuration(days: string, hours: string) {
  if (
    (days && !/^\d+$/u.test(days)) ||
    (hours && !/^\d+(?:[.,]\d+)?$/u.test(hours))
  )
    return null
  const seconds =
    Number(days) * day + Math.round(Number(hours.replace(',', '.')) * hour)
  return Number.isSafeInteger(seconds) && seconds > 0 ? seconds : null
}

export function durationInputs(seconds: number) {
  return {
    days: String(Math.floor(seconds / day)),
    hours: String(Number(((seconds % day) / hour).toFixed(6))),
  }
}

export function formatDuration(seconds: number) {
  const parts = [
    [Math.floor(seconds / day), 'д'],
    [Math.floor((seconds % day) / hour), 'ч'],
    [Math.floor((seconds % hour) / 60), 'мин'],
    [seconds % 60, 'с'],
  ] as const
  return parts
    .filter(([count]) => count > 0)
    .map(([count, unit]) => `${String(count)} ${unit}`)
    .join(' ')
}

export function basisLabel(basis: string) {
  if (basis === 'manual') return 'Вручную'
  if (basis === 'historical_p85') return 'Исторический p85'
  return 'Неизвестное основание'
}

export function targetProposal(
  draft: TargetDraft,
  etag: string,
  current?: TargetVersion,
): ChangeProposal<TargetDraft> {
  return {
    draft,
    etag,
    changes: [
      {
        label: 'Целевой срок',
        before: current ? formatDuration(current.duration_seconds) : 'Не задан',
        after: formatDuration(draft.duration_seconds),
      },
      {
        label: 'Основание',
        before: current ? basisLabel(current.basis) : 'Не задано',
        after: basisLabel(draft.basis),
      },
    ],
  }
}

function dateInput(value: string | null, fallback: string) {
  // Report links may carry RFC 3339 timestamps. Date controls show UTC days.
  if (!value || !/^\d{4}-\d{2}-\d{2}(?:T.*)?$/u.test(value)) return fallback
  const date = new Date(value)
  return Number.isFinite(date.getTime())
    ? date.toISOString().slice(0, 10)
    : fallback
}

export function readPeriodInputs(search: URLSearchParams) {
  const end = new Date()
  end.setUTCHours(0, 0, 0, 0)
  end.setUTCDate(end.getUTCDate() - ((end.getUTCDay() + 6) % 7))
  const start = new Date(end)
  start.setUTCDate(start.getUTCDate() - 12 * 7)
  return {
    from: dateInput(search.get('from'), start.toISOString().slice(0, 10)),
    to: dateInput(search.get('to'), end.toISOString().slice(0, 10)),
  }
}

export function toTargetPeriod(from: string, to: string): TargetPeriod | null {
  const valid = (date: string) => dateInput(date, '') === date && date !== ''
  if (!valid(from) || !valid(to) || from >= to) return null
  return { from: from + 'T00:00:00Z', to: to + 'T00:00:00Z' }
}

export function isAccessError(error: unknown) {
  return (
    error instanceof ApiError &&
    (error.kind === 'forbidden' ||
      error.kind === 'auth-required' ||
      error.kind === 'session-ended')
  )
}

export const durationIssue: FormIssue = {
  field: 'target-days',
  message:
    'Укажите срок больше нуля. Сутки вводятся целым числом, часы можно указать дробью.',
}

export function validationIssues({ fields }: ChangeValidation): FormIssue[] {
  const issues: FormIssue[] = []
  if (fields.has('/duration_seconds')) issues.push(durationIssue)
  if (
    ['/basis', '/basis_revision_id', '/basis_period'].some((field) =>
      fields.has(field),
    )
  ) {
    issues.push({
      field: 'target-basis',
      message:
        'Основание недоступно. Обновите ориентир или укажите срок вручную.',
    })
  }
  if (fields.has('/task_type') || fields.has('/size_label')) {
    issues.push({ field: null, message: 'Проверьте тип и размер группы.' })
  }
  return issues.length > 0
    ? issues
    : [
        {
          field: null,
          message: 'Сервер отклонил срок. Проверьте данные формы.',
        },
      ]
}
