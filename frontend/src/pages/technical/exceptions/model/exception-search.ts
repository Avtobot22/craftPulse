import type { ExceptionListParams } from '@/shared/api'

export const states = [
  'pending',
  'active',
  'rejected',
  'withdrawn',
  'expired',
] as const
export const sorts = {
  expires_at: 'Сначала ближайший срок',
  '-expires_at': 'Сначала дальний срок',
  '-created_at': 'Сначала новые',
  created_at: 'Сначала старые',
} satisfies Record<NonNullable<ExceptionListParams['sort']>, string>

export const uuid = /^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/iu

export function readExceptionSearch(
  search: URLSearchParams,
): ExceptionListParams {
  const state = states.find((value) => value === search.get('state'))
  const sort = Object.keys(sorts).find(
    (value): value is keyof typeof sorts =>
      value === search.get('sort') && Object.hasOwn(sorts, value),
  )
  const component_id = search.get('component_id')
  const finding_id = search.get('finding_id')
  const cursor = search.get('cursor')
  const limit = Number(search.get('limit'))
  return {
    ...(state ? { state } : {}),
    sort: sort ?? 'expires_at',
    ...(component_id && uuid.test(component_id) ? { component_id } : {}),
    ...(finding_id && uuid.test(finding_id) ? { finding_id } : {}),
    ...(cursor ? { cursor } : {}),
    ...(limit > 0 && limit <= 100 && Number.isInteger(limit) ? { limit } : {}),
  }
}
