import { ActivityTime, SourceLink } from '@/entities/activity'
import { contractLabel, technicalLabels } from '@/shared/config'
import type { FindingDetail } from '../model/finding-links'

const triageStates = {
  open: 'Открыта',
  resolved: 'Исправлена',
  accepted_risk: 'Принят риск',
  false_positive: 'Ложное срабатывание',
  not_applicable: 'Не применимо',
  accepted: 'Принято',
  rejected: 'Отклонено',
  underReview: 'На рассмотрении',
  suppressed: 'Подавлено',
}

export function FindingDecisions({ finding }: { finding: FindingDetail }) {
  const { external_triage, exceptions } = finding
  return (
    <>
      <section>
        <h2>Решения craftPulse</h2>
        {exceptions.length ? (
          <ul>
            {exceptions.map(({ exception_id, state, expires_at }) => (
              <li key={exception_id}>
                {contractLabel(technicalLabels.exceptionState, state)}
                {' · Срок: '}
                <ActivityTime instant={expires_at} />
              </li>
            ))}
          </ul>
        ) : (
          <p>Решений пока нет.</p>
        )}
      </section>
      <section>
        <h2>Внешний триаж</h2>
        {external_triage.length ? (
          <ul>
            {external_triage.map(({ authority, state, source_url }, index) => (
              <li key={`${authority}-${String(index)}`}>
                <SourceLink url={source_url}>{authority}</SourceLink>
                {' · '}
                {contractLabel(triageStates, state)}
              </li>
            ))}
          </ul>
        ) : (
          <p>Данные внешнего триажа отсутствуют.</p>
        )}
      </section>
    </>
  )
}
