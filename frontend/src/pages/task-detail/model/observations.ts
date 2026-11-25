import type { components, TimelineEvent } from '@/shared/api'
import type { Basis, Observation } from './task-evidence'
import { atEvidenceTime, isStatusChange } from './task-evidence'

type KnownObservation =
  components['schemas']['TaskDetail']['observations'][number]

const labels = {
  awaiting_reviewer: 'MR готов к ревью, ревьюер не назначен',
  awaiting_first_response: 'MR ждёт первого ответа человека',
  ci_failed: 'Pipeline упал',
  ci_running: 'Pipeline выполняется',
  ci_passed_after_retry: 'Pipeline успешен после повторных попыток',
  missing_report: 'Ожидаемый отчёт проверки не получен',
  new_finding: 'Новая находка в связанном MR',
  blocked: 'Задача заблокирована',
  no_merge_request: 'Подтверждённый MR не найден',
  over_target: 'Превышен целевой срок',
  unknown: 'Причина задержки не установлена',
} satisfies Record<KnownObservation['kind'], string>

const actions = {
  assign_reviewer: {
    label: 'Назначить ревьюера в GitLab',
    kind: 'merge_request',
  },
  request_review: { label: 'Запросить ревью в GitLab', kind: 'merge_request' },
  check_pipeline: { label: 'Проверить pipeline в GitLab CI', kind: 'pipeline' },
  restore_report: { label: 'Проверить отчёт в GitLab CI', kind: 'pipeline' },
  review_finding: { label: 'Проверить находку в GitLab CI', kind: 'finding' },
  resolve_blocker: { label: 'Уточнить блокировку в Jira DC', kind: 'task' },
  link_merge_request: { label: 'Проверить связь с MR в Jira DC', kind: 'task' },
  review_target: { label: 'Проверить целевой срок в craftPulse' },
  clarify_reason: { label: 'Уточнить у исполнителя в Jira DC', kind: 'task' },
} satisfies Record<KnownObservation['action'], { label: string; kind?: string }>

function lookup<T>(values: Readonly<Record<string, T>>, key: string) {
  return Object.hasOwn(values, key) ? values[key] : undefined
}

export function observationLabel(observation: Observation) {
  if (observation.question === 'reviewer_assigned') return labels.unknown
  return lookup(labels, observation.kind) ?? observation.kind
}

export function observationQuestion(observation: Observation) {
  const mr = observation.basis.find(({ kind }) => kind === 'merge_request')
  switch (observation.question) {
    case 'reviewer_assigned':
      return `Назначен ли ревьюер в MR${mr?.display_key ? ' ' + mr.display_key : ''}?`
    case 'merge_request_exists':
      return 'Есть ли MR по задаче?'
    case 'delay_reason':
      return 'Что задерживает задачу?'
    case undefined:
    default:
      if (observation.kind === 'no_merge_request')
        return 'Есть ли MR по задаче?'
      if (observation.kind === 'unknown') return 'Что задерживает задачу?'
      return undefined
  }
}

export function observationAction(observation: Observation) {
  const code =
    observation.kind === 'unknown' ||
    observation.question === 'reviewer_assigned'
      ? 'clarify_reason'
      : observation.action
  return (
    lookup<{ label: string; kind?: string }>(actions, code) ?? { label: code }
  )
}

export function observationEvents(
  observation: Observation,
  basis: Basis,
  events: readonly TimelineEvent[],
) {
  const { since, kind, question } = observation
  if (since === null) return []
  return events.filter((event) => {
    if (!atEvidenceTime(event, basis, since)) return false
    if (basis.kind === 'merge_request')
      return (
        (kind === 'awaiting_reviewer' ||
          kind === 'awaiting_first_response' ||
          question === 'reviewer_assigned') &&
        ['ready', 'reviewer_removed', 'reviewer_assigned'].includes(
          event.event_kind,
        )
      )
    if (basis.kind === 'pipeline')
      return (
        (kind === 'ci_running' && event.event_kind === 'pipeline_started') ||
        (['ci_failed', 'ci_passed_after_retry'].includes(kind) &&
          event.event_kind === 'pipeline_finished')
      )
    return kind === 'blocked' && basis.kind === 'task' && isStatusChange(event)
  })
}
