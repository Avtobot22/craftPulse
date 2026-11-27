import type { FieldError } from '@/shared/api'
import type { ChangeValidation } from '@/shared/ui'

const messages: Record<FieldError['code'], string> = {
  required: 'Заполните обязательное поле.',
  invalid_format: 'Проверьте формат значения.',
  out_of_range: 'Допустимо не более 200 назначений.',
  too_long: 'Значение слишком длинное.',
  unknown_field: 'Поле не поддерживается сервером.',
  not_allowed_value: 'Это значение недопустимо для назначения.',
  duplicate: 'Такое назначение уже есть.',
  outside_scope: 'Область недоступна.',
  unknown_reference: 'Выбранная область больше недоступна.',
  unknown: 'Проверьте значение поля.',
}

export function roleIssues(validation: ChangeValidation) {
  const issues = new Map<string, string>()
  for (const [path, errors] of validation.fields) {
    issues.set(
      path,
      [...new Set(errors.map(({ code }) => messages[code]))].join(' '),
    )
  }
  if (!issues.size) issues.set('/data', 'Сервер отклонил набор ролей.')
  return issues
}

export function roleIssueRowIndex(path: string): number | undefined {
  const match =
    /^\/data\/(\d+)(\/(role|team_id|project_id|service_id))?$/u.exec(path)
  return match ? Number(match[1]) : undefined
}
