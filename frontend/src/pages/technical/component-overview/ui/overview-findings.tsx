import { Card } from '@gravity-ui/uikit'
import { MetricValue } from '@/entities/metric'
import type { components, OpenCodes } from '@/shared/api'
import { contractLabel, routePaths, technicalLabels } from '@/shared/config'
import { DataStateBadge, InspectionLink } from '@/shared/ui'

type Count = OpenCodes<
  components['schemas']['ComponentOverview']['findings_by_state'][number]
>

function countMetric(kind: string, state: string) {
  if (kind === 'code' && state === 'open') return 'C11'
  if (
    state !== 'accepted_risk' &&
    state !== 'false_positive' &&
    state !== 'not_applicable'
  )
    return undefined
  if (kind === 'code') return 'C26'
  if (['security', 'secret', 'dependency', 'iac'].includes(kind)) return 'S35'
  return undefined
}

export function OverviewFindings({
  rows,
  search,
}: {
  rows: Count[]
  search: URLSearchParams
}) {
  return (
    <section aria-labelledby="overview-findings-heading">
      <h2 id="overview-findings-heading">Находки по видам и состояниям</h2>
      {rows.length === 0 ? (
        <DataStateBadge
          state="NOT_EVALUATED"
          reason="Нет данных о находках по отчётам"
        />
      ) : (
        <ul className="cp-component-overview__cards">
          {rows.map(({ finding_kind, state, count }) => {
            const id = `overview-count-${finding_kind}-${state}`
            const label = `${contractLabel(technicalLabels.findingKind, finding_kind)} · ${contractLabel(technicalLabels.findingState, state)}`
            const metric = countMetric(finding_kind, state)
            const filter = new URLSearchParams(search)
            filter.set('finding_kind', finding_kind)
            filter.set('state', state)
            const canDrillDown =
              Object.hasOwn(technicalLabels.findingKind, finding_kind) &&
              Object.hasOwn(technicalLabels.findingState, state) &&
              (((count.state === 'ASSESSED' || count.state === 'PROVISIONAL') &&
                count.value !== null) ||
                (count.state === 'BOUNDED' &&
                  (count.bound?.kind === 'lower' ||
                    count.bound?.kind === 'upper')))
            return (
              <li key={id}>
                <Card className="cp-component-overview__card">
                  <h3>
                    {label}
                    {metric && ` (${metric})`}
                  </h3>
                  <MetricValue
                    value={count}
                    {...(canDrillDown
                      ? {
                          renderValue: (display) => (
                            <InspectionLink
                              id={id}
                              to={{
                                pathname: routePaths.findings,
                                search: filter.toString(),
                              }}
                            >
                              {display}
                              <span className="cp-component-overview__sr-only">
                                {' '}
                                · {label}
                              </span>
                            </InspectionLink>
                          ),
                        }
                      : {})}
                  />
                </Card>
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}
