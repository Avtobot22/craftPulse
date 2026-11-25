import { Button, Text } from '@gravity-ui/uikit'
import {
  Link,
  useLocation,
  useNavigate,
  useParams,
  useSearchParams,
} from 'react-router'
import { routePaths } from '@/shared/config'

export function MetricPassportPage() {
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
