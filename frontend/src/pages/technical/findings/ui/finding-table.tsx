import { generatePath, useSearchParams } from 'react-router'
import { ActivityTime } from '@/entities/activity'
import { formatFindingLocation } from '@/entities/finding'
import type { components, OpenCodes } from '@/shared/api'
import { contractLabel, routePaths, technicalLabels } from '@/shared/config'
import { InspectionLink } from '@/shared/ui'
import type { ReactNode } from 'react'

type Finding = OpenCodes<components['schemas']['Finding']>

const columns = [
  'Правило',
  'Место',
  'Серьёзность',
  'Состояние',
  'Впервые замечена',
] as const

function Cell({
  column,
  children,
}: {
  column: (typeof columns)[number]
  children: ReactNode
}) {
  return (
    <td role="cell">
      <span className="cp-findings__mobile-label" aria-hidden="true">
        {column}
      </span>
      {children}
    </td>
  )
}

export function FindingTable({
  findings,
  pageCount,
}: {
  findings: readonly Finding[]
  pageCount: number
}) {
  const [search] = useSearchParams()
  const scope = new URLSearchParams()
  for (const key of ['org_slug', 'team_id', 'project_id']) {
    const value = search.get(key)
    if (value) scope.set(key, value)
  }

  return (
    <div className="cp-findings__scroll">
      <table role="table" aria-label="Находки">
        <thead role="rowgroup">
          <tr role="row">
            {columns.map((column) => (
              <th role="columnheader" key={column} scope="col">
                {column}
              </th>
            ))}
          </tr>
        </thead>
        <tbody role="rowgroup">
          {findings.map(
            ({
              id,
              rule_id,
              finding_kind,
              location,
              package: pkg,
              severity,
              state,
              first_seen_at,
            }) => (
              <tr key={id} role="row">
                <Cell column="Правило">
                  <InspectionLink
                    id={`findings-row-${id}`}
                    pageCount={pageCount}
                    to={{
                      pathname: generatePath(routePaths.findingDetail, {
                        findingId: encodeURIComponent(id),
                      }),
                      search: scope.toString(),
                    }}
                  >
                    <code>{rule_id}</code>
                  </InspectionLink>
                  <div>
                    {contractLabel(technicalLabels.findingKind, finding_kind)}
                  </div>
                </Cell>
                <Cell column="Место">
                  {location ? (
                    <code>{formatFindingLocation(location)}</code>
                  ) : pkg && finding_kind !== 'secret' ? (
                    <code>
                      {pkg.purl}
                      {pkg.version && ` · ${pkg.version}`}
                    </code>
                  ) : (
                    'неизвестно'
                  )}
                </Cell>
                <Cell column="Серьёзность">
                  {contractLabel(
                    technicalLabels.severity,
                    severity.mapped ?? 'unknown',
                  )}
                  <div className="cp-findings__secondary">
                    Исходная: {severity.native ?? 'неизвестно'} · версия
                    сопоставления: {severity.mapping_version ?? 'неизвестно'}
                  </div>
                </Cell>
                <Cell column="Состояние">
                  {contractLabel(technicalLabels.findingState, state)}
                </Cell>
                <Cell column="Впервые замечена">
                  <ActivityTime instant={first_seen_at} />
                </Cell>
              </tr>
            ),
          )}
        </tbody>
      </table>
    </div>
  )
}
