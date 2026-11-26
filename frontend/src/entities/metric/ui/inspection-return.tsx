import { Button } from '@gravity-ui/uikit'
import { Link, useLocation, useNavigate } from 'react-router'
import { routePaths } from '@/shared/config'
import type { MetricInspection } from '../model/inspection'

export function reportHistoryDepth(state: unknown) {
  if (state && typeof state === 'object') {
    if ('reportHistoryDepth' in state && state.reportHistoryDepth === 2)
      return 2
    if ('fromTeamReport' in state && state.fromTeamReport === true) return 1
  }
  return 0
}

export function InspectionReturn({
  selection,
}: {
  selection: MetricInspection
}) {
  const location = useLocation()
  const navigate = useNavigate()
  const depth = reportHistoryDepth(location.state)
  if (depth) {
    return (
      <Button size="xl" onClick={() => void navigate(-depth)}>
        Вернуться к отчёту
      </Button>
    )
  }

  const { search: details } = selection
  const search = new URLSearchParams(details.get('report_search') ?? details)
  const focusTarget = details.get('focus_target')
  const focusSeries = details.get('focus_series')
  const focusCard = details.get('focus_card')
  for (const key of [
    'point_start',
    'estimator',
    'group_value',
    'role',
    'focus_target',
    'focus_card',
    'focus_series',
    'report_search',
  ])
    search.delete(key)
  search.set('revision_id', selection.revisionId)
  return (
    <Link
      to={{ pathname: routePaths.teamReport, search: search.toString() }}
      state={
        focusTarget
          ? {
              focusTarget,
              focusSeries,
              pointStart: selection.point,
              seriesId: focusCard,
            }
          : undefined
      }
    >
      Вернуться к отчёту
    </Link>
  )
}
