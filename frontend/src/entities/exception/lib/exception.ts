import type { QueryClient } from '@tanstack/react-query'
import { generatePath } from 'react-router'
import type { ExceptionScope } from '@/shared/api'
import { contractLabel, routePaths, technicalLabels } from '@/shared/config'

const uuid = /^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/iu
const dateFormat = new Intl.DateTimeFormat('ru-RU', {
  dateStyle: 'medium',
  timeStyle: 'short',
})

export function formatExceptionDate(value: string) {
  const instant = Date.parse(value)
  return Number.isFinite(instant)
    ? dateFormat.format(instant)
    : 'Дата недоступна'
}

export function describeExceptionScope(scope: ExceptionScope) {
  return [
    ['Находка', scope.finding_id],
    ['Правило', scope.rule_id],
    ['Путь', scope.path_prefix],
    ['Компонент', scope.component_id],
    ['Пакет', scope.package_purl],
    ['Область', scope.area_id],
  ]
    .filter(([, value]) => value)
    .map(([label, value]) => `${label ?? ''}: ${value ?? ''}`)
    .join(' · ')
}

export function exceptionOrigin(
  search: URLSearchParams,
  findingId?: string | null,
) {
  const id = search.get('origin_finding') ?? findingId
  if (!id || !uuid.test(id)) return undefined
  const params = new URLSearchParams(search)
  for (const key of [
    'origin_finding',
    'finding_id',
    'component_id',
    'state',
    'cursor',
    'sort',
    'limit',
  ])
    params.delete(key)
  return {
    pathname: generatePath(routePaths.findingDetail, { findingId: id }),
    search: params.toString(),
  }
}

export function exceptionStatus(state: string) {
  return state === 'pending'
    ? 'Ждёт подтверждения ревьюером'
    : contractLabel(technicalLabels.exceptionState, state)
}

export function invalidateExceptionViews(queryClient: QueryClient) {
  for (const key of ['exceptions', 'findings'])
    void queryClient.invalidateQueries({ queryKey: [key] })
}
