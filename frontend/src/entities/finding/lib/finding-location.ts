import type { components } from '@/shared/api'

export function formatFindingLocation(
  location: NonNullable<components['schemas']['CodeLocation']>,
) {
  const first = location.line_start ?? location.line_end
  const last = location.line_end
  return `${location.path}${first === null ? '' : `:${String(first)}`}${last !== null && last !== first ? `-${String(last)}` : ''}`
}
