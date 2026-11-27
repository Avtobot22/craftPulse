import type { FindingListParams } from '@/shared/api'
import { technicalLabels } from '@/shared/config'

export const findingSorts = {
  '-severity': 'Сначала высокая серьёзность',
  severity: 'Сначала низкая серьёзность',
  '-first_seen_at': 'Сначала новые',
  first_seen_at: 'Сначала ранние',
} satisfies Record<NonNullable<FindingListParams['sort']>, string>

export const scopeFilters = {
  component_id: 'Компонент',
  project_id: 'Проект',
  service_id: 'Сервис',
} as const
export const scopeFilterKeys = [
  'component_id',
  'project_id',
  'service_id',
] as const

export const idPattern = '[0-9A-Fa-f]{8}(?:-[0-9A-Fa-f]{4}){3}-[0-9A-Fa-f]{12}'
const uuid = new RegExp(`^${idPattern}$`, 'u')

function readOption<T extends string>(
  search: URLSearchParams,
  name: string,
  options: Record<T, string>,
): T | undefined {
  const value = search.get(name)
  return value && Object.hasOwn(options, value) ? (value as T) : undefined
}

export function readFindingSearch(search: URLSearchParams): FindingListParams {
  const params: FindingListParams = {}
  for (const key of scopeFilterKeys) {
    const value = search.get(key)
    if (value && uuid.test(value)) params[key] = value
  }
  const kind = readOption(search, 'finding_kind', technicalLabels.findingKind)
  const state = readOption(search, 'state', technicalLabels.findingState)
  const severity = readOption(search, 'severity', technicalLabels.severity)
  const sort = readOption(search, 'sort', findingSorts)
  const cursor = search.get('cursor')
  const rawLimit = search.get('limit')
  const limit =
    rawLimit && /^[0-9]{1,3}$/u.test(rawLimit) ? Number(rawLimit) : 0
  if (kind) params.finding_kind = kind
  if (state) params.state = state
  if (severity) params.severity = severity
  if (sort) params.sort = sort
  if (cursor) params.cursor = cursor
  if (limit >= 1 && limit <= 200) params.limit = limit
  return params
}
