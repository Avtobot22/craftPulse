import { generatePath } from 'react-router'
import type { components, OpenCodes } from '@/shared/api'
import { routePaths, technicalLabels } from '@/shared/config'

type Item = OpenCodes<components['schemas']['SummaryItem']>

export function summaryComparisonLink(
  search: URLSearchParams,
  item: Item | undefined,
) {
  const params = new URLSearchParams(search)
  let pathname: string = routePaths.technicalComparisonSetup
  const kind = item?.analysis_kind
  if (item && kind && Object.hasOwn(technicalLabels.analysisKind, kind)) {
    params.set('analysis_kind', kind)
    if (item.component_id) {
      pathname = generatePath(routePaths.technicalComparison, {
        componentId: encodeURIComponent(item.component_id),
      })
    } else if (item.finding_id) {
      params.set('finding_id', item.finding_id)
    }
  }
  return { pathname, search: params.toString() }
}
