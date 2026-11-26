import { Button, Text } from '@gravity-ui/uikit'
import {
  Link,
  useLocation,
  useNavigate,
  useOutletContext,
  useParams,
  useSearchParams,
} from 'react-router'
import { isDateTime } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import {
  periodError,
  readReportParams,
  reportQuery,
  routePaths,
} from '@/shared/config'
import type { TeamReportScope } from '@/shared/config'
import { ReportPointPassport } from './report-point-passport'

export function MetricPassportPage({ api }: { api: ApiClient }) {
  const { metricId, revisionId } = useParams()
  const location = useLocation()
  const navigate = useNavigate()
  const [search] = useSearchParams()
  const state: unknown = location.state
  const inReportHistory =
    state &&
    typeof state === 'object' &&
    'fromTeamReport' in state &&
    state.fromTeamReport === true
  const point = search.get('point_start')
  const { team, project } = useOutletContext<TeamReportScope>()
  const params = readReportParams(search)
  const hasReportPeriod = search.has('from') && search.has('to')
  const reportSearch = new URLSearchParams(search)
  reportSearch.delete('point_start')
  reportSearch.delete('estimator')
  reportSearch.delete('group_value')
  if (revisionId) reportSearch.set('revision_id', revisionId)
  return (
    <section>
      <Text as="h1" variant="header-1">
        Паспорт показателя
      </Text>
      {point && (
        <p>
          {metricId} · <time dateTime={point}>{point}</time>
        </p>
      )}
      {point &&
        revisionId &&
        metricId &&
        (team &&
        hasReportPeriod &&
        isDateTime(point) &&
        !periodError(params) ? (
          <ReportPointPassport
            api={api}
            teamId={team.id}
            revisionId={revisionId}
            metricId={metricId}
            pointStart={point}
            query={reportQuery(
              { ...params, revision: revisionId },
              project?.id,
            )}
            estimator={search.get('estimator')}
            groupValue={search.get('group_value')}
          />
        ) : (
          <p role="status">Не удалось определить срез выбранной точки.</p>
        ))}
      {inReportHistory ? (
        <Button size="xl" onClick={() => void navigate(-1)}>
          Вернуться к отчёту
        </Button>
      ) : (
        point && (
          <Link
            to={{
              pathname: routePaths.teamReport,
              search: reportSearch.toString(),
            }}
          >
            Вернуться к отчёту
          </Link>
        )
      )}
    </section>
  )
}
