import { generatePath } from 'react-router'
import { CommitSha } from '@/entities/check-report'
import { RestrictedResults } from '@/entities/result-access'
import type { components, OpenCodes } from '@/shared/api'
import { contractLabel, routePaths, technicalLabels } from '@/shared/config'
import { InspectionLink } from '@/shared/ui'

type Comparison = OpenCodes<components['schemas']['TechnicalComparison']>

export function ComparisonResult({ comparison }: { comparison: Comparison }) {
  const { base, head, comparable, incomparable_reasons, factors } = comparison

  return (
    <>
      <div className="cp-comparison-reports">
        {[
          { label: 'База', report: base },
          { label: 'Head', report: head },
        ].map(({ label, report }) => (
          <section key={label}>
            <h2>{label}</h2>
            <CommitSha sha={report.sha} />
            <p>
              {report.report_id ? (
                <>
                  Отчёт <code>{report.report_id}</code>
                </>
              ) : (
                'Отчёт отсутствует'
              )}
            </p>
          </section>
        ))}
      </div>
      <RestrictedResults
        access_limited={comparison.access_limited}
        access_notes={comparison.access_notes}
      />
      {!comparable ? (
        <section>
          <h2>Результаты несопоставимы</h2>
          {incomparable_reasons.length === 0 && <p>Причина не указана.</p>}
          <ul>
            {incomparable_reasons.map((reason) => (
              <li key={reason}>
                {contractLabel(technicalLabels.incomparableReason, reason)}
              </li>
            ))}
          </ul>
        </section>
      ) : (
        <section>
          <h2>Факторы изменения</h2>
          {factors.length === 0 && <p>Нет доступных факторов изменения.</p>}
          {factors.map((factor) => (
            <section key={factor.factor}>
              <h3>
                {contractLabel(technicalLabels.comparisonFactor, factor.factor)}
              </h3>
              <dl>
                <dt>Новые</dt>
                <dd>{factor.new_count ?? 'Нет данных'}</dd>
                <dt>Устранённые</dt>
                <dd>{factor.resolved_count ?? 'Нет данных'}</dd>
              </dl>
              {factor.finding_ids.length > 0 && (
                <ul>
                  {factor.finding_ids.map((id) => (
                    <li key={id}>
                      <InspectionLink
                        id={`factor-${factor.factor}-${id}`}
                        to={generatePath(routePaths.findingDetail, {
                          findingId: encodeURIComponent(id),
                        })}
                      >
                        Находка {id}
                      </InspectionLink>
                    </li>
                  ))}
                </ul>
              )}
            </section>
          ))}
        </section>
      )}
    </>
  )
}
