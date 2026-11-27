import { ApiError } from '@/shared/api'
import type { Adapter, ConnectionDraft, Credentials } from './connection-draft'
import { scopeKinds } from './connection-draft'

export interface Issue {
  field?: string
  message: string
}

const outsideScope = 'Адрес вне разрешённой сетевой области установки'
const scopeConflict = 'Эту область уже собирает другое активное подключение'

function isHttps(value: string) {
  if (!/^https:\/\/[^/]/u.test(value) || /[\s\\]/u.test(value)) return false
  const url = URL.parse(value)
  return Boolean(url?.hostname && !url.username && !url.password)
}

export function credentialPath(name: string) {
  return '/credentials/' + name.replaceAll('~', '~0').replaceAll('/', '~1')
}

export function formIssues(
  draft: ConnectionDraft,
  adapter: Adapter | undefined,
  credentials: Credentials | undefined,
): Issue[] {
  const errors: Issue[] = []
  if (!adapter) errors.push({ field: '/provider', message: 'Выберите адаптер' })
  if (!draft.name.trim() || draft.name.length > 200)
    errors.push({
      field: '/name',
      message: 'Укажите имя длиной от 1 до 200 символов',
    })
  if (!isHttps(draft.base_url) || draft.base_url.length > 2048)
    errors.push({
      field: '/base_url',
      message: 'Укажите адрес HTTPS длиной до 2048 символов',
    })
  if (!draft.scopes.length || draft.scopes.length > 500)
    errors.push({ field: '/scopes', message: 'Добавьте от 1 до 500 областей' })

  const kinds = scopeKinds(adapter)
  draft.scopes.forEach((row, index) => {
    const path = '/scopes/' + String(index)
    if (!kinds.some((kind) => kind === row.kind))
      errors.push({ field: path + '/kind', message: 'Выберите вид области' })
    if (!row.native_scope_id.trim() || row.native_scope_id.length > 255)
      errors.push({
        field: path + '/native_scope_id',
        message: 'Укажите идентификатор длиной от 1 до 255 символов',
      })
    if (
      row.history_from !== null &&
      !Number.isFinite(Date.parse(row.history_from))
    )
      errors.push({
        field: path + '/history_from',
        message: 'Укажите начало истории или выберите всю доступную историю',
      })
  })

  if (credentials) {
    for (const { name, required } of adapter?.credential_fields ?? []) {
      const value = Object.hasOwn(credentials, name)
        ? credentials[name]
        : undefined
      if (required && !value?.trim())
        errors.push({
          field: credentialPath(name),
          message: 'Заполните обязательное поле',
        })
      if (value && value.length > 8192)
        errors.push({
          field: credentialPath(name),
          message: 'Введите до 8192 символов',
        })
    }
    if (Object.keys(credentials).length > 16)
      errors.push({
        field: '/credentials',
        message: 'Допускается не больше 16 полей учётных данных',
      })
  }
  return errors
}

const fieldMessages: Record<string, string> = {
  required: 'Заполните обязательное поле',
  invalid_format: 'Проверьте формат значения',
  out_of_range: 'Значение вне допустимого диапазона',
  too_long: 'Значение слишком длинное',
  unknown_field: 'Поле не поддерживается',
  not_allowed_value: 'Выберите допустимое значение',
  duplicate: 'Такое значение уже указано',
  outside_scope: 'Значение вне разрешённой области',
  unknown_reference: 'Значение не найдено',
}

export function writeFailure(error: unknown, fields: ReadonlySet<string>) {
  const problem = error instanceof ApiError ? error.problem : null
  if (problem?.status === 412) return { kind: 'conflict' } as const
  if (problem?.status === 409 && problem.code === 'state_conflict') {
    const field = problem.errors?.find(
      ({ field }) => field.startsWith('/scopes/') && fields.has(field),
    )?.field
    return {
      kind: 'issues',
      issues: [{ field: field ?? '/scopes', message: scopeConflict }],
    } as const
  }
  const issues: Issue[] = []
  if (problem?.status === 422) {
    if (problem.code === 'outside_scope')
      issues.push({ field: '/base_url', message: outsideScope })
    for (const { field, code } of problem.errors ?? []) {
      if (field === '/base_url' && code === 'outside_scope') {
        if (!issues.some(({ message }) => message === outsideScope))
          issues.push({ field, message: outsideScope })
        continue
      }
      const message = fieldMessages[code] ?? 'Проверьте значение'
      issues.push(fields.has(field) ? { field, message } : { message })
    }
  }
  if (!issues.length) {
    issues.push({
      message:
        error instanceof ApiError && error.kind === 'missing-version'
          ? 'Версия подключения недоступна. Обновите страницу перед сохранением'
          : 'Не удалось сохранить подключение',
    })
  }
  return { kind: 'issues', issues } as const
}

export function fieldId(path: string) {
  return (
    'connection-field' +
    encodeURIComponent(path).replaceAll('_', '_5F').replaceAll('%', '_')
  )
}

export function editablePaths(
  draft: ConnectionDraft,
  adapter: Adapter | undefined,
  editCredentials: boolean,
) {
  const fields = new Set([
    '/provider',
    '/name',
    '/base_url',
    '/scopes',
    '/credentials',
  ])
  draft.scopes.forEach((_, index) => {
    const path = '/scopes/' + String(index)
    for (const suffix of ['', '/kind', '/native_scope_id', '/history_from'])
      fields.add(path + suffix)
  })
  if (editCredentials)
    for (const { name } of adapter?.credential_fields ?? [])
      fields.add(credentialPath(name))
  return fields
}
