import { Button, Text } from '@gravity-ui/uikit'
import {
  Link,
  Navigate,
  useLocation,
  useNavigate,
  useSearchParams,
} from 'react-router'
import {
  InspectionReturn,
  isRevisionAccessDenied,
  metricDetailsLink,
  reportHistoryDepth,
  useMetricInspection,
  useMetricRevision,
} from '@/entities/metric'
import type { ApiClient } from '@/shared/api'
import { routePaths } from '@/shared/config'
import { ErrorState, LoadingState } from '@/shared/ui'
import { parseRole, roleLabels } from '../api/cohort-api'
import { CohortResults } from './cohort-results'
import './metric-cohort-page.css'

export function MetricCohortPage({ api }: { api: ApiClient }) {
  const selection = useMetricInspection()
  const location = useLocation()
  const navigate = useNavigate()
  const navigationState: unknown = location.state
  const fromPassport = reportHistoryDepth(navigationState) === 2
  const [, setSearch] = useSearchParams()
  const revision = useMetricRevision(api, selection.revisionId)
  const data = revision.isError ? undefined : revision.data
  const definition = data?.definitions.find(
    ({ metric_id }) => metric_id === selection.metricId,
  )

  if (isRevisionAccessDenied(revision.error))
    return <Navigate to={routePaths.unavailable} replace />

  return (
    <section className="cp-cohort">
      <Text as="h1" variant="header-1">
        Когорта точки
      </Text>
      <nav aria-label="Навигация по показателю">
        <InspectionReturn selection={selection} />
        {fromPassport ? (
          <Button size="xl" onClick={() => void navigate(-1)}>
            Паспорт показателя
          </Button>
        ) : (
          <Link to={metricDetailsLink(selection, 'metricPassport')}>
            Паспорт показателя
          </Link>
        )}
      </nav>
      {!selection.validPoint ? (
        <p role="alert">Укажите дату начала точки.</p>
      ) : revision.isPending ? (
        <LoadingState />
      ) : revision.isError ? (
        <ErrorState
          reason="Не удалось загрузить ревизию"
          onRetry={() => void revision.refetch()}
          retrying={revision.isFetching}
        />
      ) : (
        data && (
          <>
            <p>
              {definition?.name ?? selection.metricId} · Ревизия{' '}
              {data.revision_no} ·{' '}
              <time dateTime={selection.point}>{selection.point}</time>
            </p>
            {data.loading && (
              <p role="status">Данные ревизии ещё загружаются.</p>
            )}
            {data.published_at ? (
              <>
                <label className="cp-cohort__filter">
                  Роль в формуле
                  <select
                    value={parseRole(selection.search.get('role')) ?? ''}
                    onChange={(event) => {
                      const search = new URLSearchParams(selection.search)
                      const role = parseRole(event.currentTarget.value)
                      if (role) search.set('role', role)
                      else search.delete('role')
                      setSearch(search, {
                        replace: true,
                        state: navigationState,
                      })
                    }}
                  >
                    <option value="">Все роли</option>
                    {Object.entries(roleLabels).map(([role, label]) => (
                      <option value={role} key={role}>
                        {label}
                      </option>
                    ))}
                  </select>
                </label>
                <CohortResults
                  key={JSON.stringify([
                    data.id,
                    selection.metricId,
                    selection.point,
                    selection.search.toString(),
                  ])}
                  api={api}
                  selection={selection}
                  revision={data}
                />
              </>
            ) : (
              !data.loading && <p>Ревизия не опубликована.</p>
            )}
          </>
        )
      )}
    </section>
  )
}
