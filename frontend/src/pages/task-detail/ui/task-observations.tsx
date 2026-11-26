import {
  ActivityTime,
  activitySourceName,
  SourceLink,
} from '@/entities/activity'
import type { TimelineEvent } from '@/shared/api'
import { contractLabel, contractLabels } from '@/shared/config'
import { EmptyState } from '@/shared/ui'
import {
  observationAction,
  observationEvents,
  observationLabel,
  observationQuestion,
} from '../model/observations'
import type { Basis, Observation, TaskDetail } from '../model/task-evidence'
import { basisLabel } from '../model/task-evidence'

function BasisItem({
  basis,
  observation,
  events,
}: {
  basis: Basis
  observation: Observation
  events: readonly TimelineEvent[]
}) {
  const system = activitySourceName(basis.kind)
  const label = basisLabel(basis) || 'Объект'
  const evidence = observationEvents(observation, basis, events)
  return (
    <li>
      <SourceLink url={basis.source_url}>
        {[label, system].filter(Boolean).join(' · ')}
      </SourceLink>
      {basis.kind === 'finding' && <span> (косвенная связь через MR)</span>}
      {evidence.length > 0 && (
        <ul>
          {evidence.map((event) => (
            <li key={event.id}>
              <SourceLink url={event.object.source_url}>
                {contractLabel(contractLabels.eventKind, event.event_kind)}
                {system && ` · ${system}`}
              </SourceLink>
              {' · '}
              <ActivityTime instant={event.event_time} />
            </li>
          ))}
        </ul>
      )}
    </li>
  )
}

export function TaskObservations({
  task,
  events,
}: {
  task: TaskDetail
  events: readonly TimelineEvent[]
}) {
  return (
    <section aria-labelledby="task-observations">
      <h2 id="task-observations">Наблюдения</h2>
      {task.observations.length === 0 ? (
        <EmptyState
          reason="history"
          description="Доступных наблюдений пока нет."
        />
      ) : (
        <ul className="cp-task-detail__cards">
          {task.observations.map((observation, index) => {
            const question = observationQuestion(observation)
            const action = observationAction(observation)
            const actionBasis = observation.basis.find(
              (basis) => basis.kind === action.kind && basis.source_url,
            )
            const actionUrl =
              actionBasis?.source_url ??
              (action.kind === 'task' ? task.source_url : null)
            const subject = observation.basis[0]
            return (
              <li key={`${observation.kind}-${String(index)}`}>
                <h3>{observationLabel(observation)}</h3>
                {subject && <p>{basisLabel(subject)}</p>}
                <p>
                  Начало{' '}
                  {observation.since ? (
                    <ActivityTime instant={observation.since} />
                  ) : (
                    'неизвестно'
                  )}
                </p>
                {question && (
                  <p>
                    <strong>{question}</strong>
                  </p>
                )}
                {observation.basis.length > 0 ? (
                  <>
                    <p>{question ? 'Проверенные факты' : 'Основания'}</p>
                    <ul>
                      {observation.basis.map((basis, index) => (
                        <BasisItem
                          key={`${basis.kind}-${basis.id}-${String(index)}`}
                          basis={basis}
                          observation={observation}
                          events={events}
                        />
                      ))}
                    </ul>
                  </>
                ) : (
                  <p>Доступных оснований нет</p>
                )}
                <p>
                  {observation.action === 'review_target' && !action.kind ? (
                    <a
                      href="#task-timing"
                      onClick={(event) => {
                        const target = document.getElementById('task-timing')
                        if (target) {
                          event.preventDefault()
                          target.focus()
                        }
                      }}
                    >
                      {action.label}
                    </a>
                  ) : (
                    <SourceLink url={actionUrl}>{action.label}</SourceLink>
                  )}
                </p>
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}
