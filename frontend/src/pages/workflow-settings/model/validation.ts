import type { MappingDraft, CalendarDraft } from '../api/workflow-responses'
import type { FieldError } from '@/shared/api'

export type FieldIssues = ReadonlyMap<string, string>

function timezoneError(timezone: string) {
  if (!timezone) return 'Укажите часовой пояс IANA'
  try {
    new Intl.DateTimeFormat('ru', { timeZone: timezone })
    if (!/^[+-]/u.test(timezone)) return undefined
  } catch {
    // The server checks the same field against the database time zone catalog.
  }
  return 'Укажите известный часовой пояс IANA'
}

export function validateMapping(draft: MappingDraft): FieldIssues {
  const errors = new Map<string, string>()
  const zone = timezoneError(draft.timezone)
  if (zone) errors.set('/timezone', zone)
  draft.rules.forEach((rule, index) => {
    if (rule.rule_kind === 'size_value' && rule.rank === null)
      errors.set(`/rules/${String(index)}/rank`, 'Укажите порядок размеров')
  })
  return errors
}

export function validateCalendar(draft: CalendarDraft): FieldIssues {
  const errors = new Map<string, string>()
  const zone = timezoneError(draft.timezone)
  if (zone) errors.set('/timezone', zone)
  if (draft.work_start && !draft.work_end)
    errors.set('/work_end', 'Укажите конец рабочего дня')
  if (draft.work_end && !draft.work_start)
    errors.set('/work_start', 'Укажите начало рабочего дня')
  if (draft.work_start && draft.work_end && draft.work_end <= draft.work_start)
    errors.set('/work_end', 'Конец дня должен быть позже начала')
  const dates = new Set<string>()
  draft.holidays.forEach((day, index) => {
    if (!day) errors.set(`/holidays/${String(index)}`, 'Укажите дату праздника')
    else if (dates.has(day))
      errors.set(`/holidays/${String(index)}`, 'Дата уже добавлена')
    dates.add(day)
  })
  return errors
}

const fieldMessages: Record<FieldError['code'], string> = {
  required: 'Заполните поле',
  invalid_format: 'Проверьте формат',
  out_of_range: 'Значение вне допустимого диапазона',
  too_long: 'Значение слишком длинное',
  unknown_field: 'Неизвестное поле',
  not_allowed_value: 'Недопустимое значение',
  duplicate: 'Значение уже добавлено',
  outside_scope: 'Значение не относится к команде',
  unknown_reference: 'Значение не найдено в источнике',
  unknown: 'Проверьте значение',
}

export function validationIssues(
  fields: ReadonlyMap<string, readonly FieldError[]>,
): FieldIssues {
  const errors = new Map<string, string>()
  for (const [path, entries] of fields) {
    const entry = entries[0]
    if (!entry) continue
    const message =
      path === '/timezone'
        ? 'Проверьте часовой пояс IANA'
        : path.endsWith('/rank') && entry.code === 'required'
          ? 'Укажите порядок размеров'
          : fieldMessages[entry.code]
    errors.set(path, message)
  }
  if (!errors.size) errors.set('', 'Проверьте введённые данные')
  return errors
}
