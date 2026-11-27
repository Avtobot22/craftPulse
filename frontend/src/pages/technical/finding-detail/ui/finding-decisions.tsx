import { generatePath, useSearchParams } from 'react-router'
import { ActivityTime, SourceLink } from '@/entities/activity'
import { contractLabel, routePaths, technicalLabels } from '@/shared/config'
import { InspectionLink } from '@/shared/ui'
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
  const [search] = useSearchParams()
  const historySearch = new URLSearchParams(search)
  historySearch.set('origin_finding', finding.id)
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
                {' · '}
                <InspectionLink
                  id={`finding-exception-${exception_id}`}
                  to={{
                    pathname: generatePath(routePaths.exceptions, {
                      exceptionId: exception_id,
                    }),
                    search: historySearch.toString(),
                  }}
                >
                  История решения
                </InspectionLink>
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
