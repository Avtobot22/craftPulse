import { generatePath } from 'react-router'
import { RestrictedResults } from '@/entities/result-access'
import type { components, OpenCodes } from '@/shared/api'
import type { CurrentUser } from '@/shared/auth'
import {
  baselineContext,
  readTechnicalId,
  routePaths,
  technicalScope,
} from '@/shared/config'
import { InspectionLink } from '@/shared/ui'
import { OverviewFindings } from './overview-findings'
import { OverviewCodeFacts } from './overview-code-facts'

export function OverviewDetails({
  overview,
  search,
  current,
}: {
  overview: OpenCodes<components['schemas']['ComponentOverview']>
  search: URLSearchParams
  current: CurrentUser
}) {
  const { component } = overview
  const service = current.areas.services.find(
    ({ id }) => id === component.service_id,
  )
  const actions = current.actions
  const scope = technicalScope(search)
  const projectId = readTechnicalId(scope.get('project_id')) ?? null
  const findingsSearch = new URLSearchParams(scope)
  // The overview API has no project filter. Keep the detail count component-scoped.
  findingsSearch.delete('project_id')
  findingsSearch.set('component_id', component.id)

  const comparisonSearch = new URLSearchParams(scope)
  for (const key of ['base_sha', 'head_sha', 'analysis_kind']) {
    const value = search.get(key)
    if (value) comparisonSearch.set(key, value)
  }

  return (
    <>
      <dl className="cp-component-overview__context">
        <div>
          <dt>Компонент</dt>
          <dd>
            <code>{component.id}</code>
          </dd>
        </div>
        <div>
          <dt>Сервис</dt>
          <dd>{service?.name ?? <code>{component.service_id}</code>}</dd>
        </div>
        <div>
          <dt>Репозиторий</dt>
          <dd>
            <code>{component.repository_area_id}</code>
          </dd>
        </div>
        <div>
          <dt>Путь в репозитории (path_prefix)</dt>
          <dd>
            {component.path_prefix ? (
              <code>{component.path_prefix}</code>
            ) : (
              'Корень репозитория'
            )}
          </dd>
        </div>
      </dl>
      <RestrictedResults
        access_limited={overview.access_limited}
        access_notes={overview.access_notes}
        redactions={overview.redactions}
      />
      <nav
        className="cp-component-overview__links"
        aria-label="Данные компонента"
      >
        <InspectionLink
          id="component-findings"
          to={{
            pathname: routePaths.findings,
            search: findingsSearch.toString(),
          }}
        >
          Все находки компонента
        </InspectionLink>
        <InspectionLink
          id="component-comparison"
          to={{
            pathname: generatePath(routePaths.technicalComparison, {
              componentId: component.id,
            }),
            search: comparisonSearch.toString(),
          }}
        >
          Причины изменения результата
        </InspectionLink>
        {actions.includes('check_profiles.manage') && (
          <InspectionLink
            id="component-profile"
            to={{
              pathname: generatePath(routePaths.checkProfile, { projectId }),
              search: scope.toString(),
            }}
          >
            Матрица проверок
          </InspectionLink>
        )}
        {actions.includes('ci.read') && (
          <InspectionLink
            id="component-baselines"
            to={{
              pathname: generatePath(routePaths.baselines, { projectId }),
              search: baselineContext(search).toString(),
            }}
          >
            Базовые линии
          </InspectionLink>
        )}
      </nav>
      <OverviewFindings
        rows={overview.findings_by_state}
        search={findingsSearch}
      />
      <OverviewCodeFacts facts={overview.facts} />
    </>
  )
}
