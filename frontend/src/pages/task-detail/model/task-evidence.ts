import type { components, OpenCodes, TimelineEvent } from '@/shared/api'

export type TaskDetail = OpenCodes<components['schemas']['TaskDetail']>
export type Observation = TaskDetail['observations'][number]
export type Basis = Observation['basis'][number]

export function basisLabel(basis: Basis) {
  return [basis.display_key, basis.title].filter(Boolean).join(' · ')
}

export function atEvidenceTime(
  event: TimelineEvent,
  object: Pick<Basis, 'id' | 'kind'>,
  instant: string,
) {
  return (
    event.object.id === object.id &&
    event.object.kind === object.kind &&
    Date.parse(event.event_time) === Date.parse(instant)
  )
}

export function isStatusChange(event: TimelineEvent) {
  return event.event_kind === 'field_change' && event.field === 'status'
}
