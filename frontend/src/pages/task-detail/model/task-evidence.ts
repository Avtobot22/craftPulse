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

export function blockStartEvent(
  taskId: string,
  start: string,
  events: readonly TimelineEvent[],
) {
  const changes = events.filter(
    (event) =>
      event.event_kind === 'field_change' &&
      atEvidenceTime(event, { kind: 'task', id: taskId }, start),
  )
  // A blocking interval can start with a status change or a mapped tracker flag.
  return changes.length === 1 ? changes[0] : undefined
}
