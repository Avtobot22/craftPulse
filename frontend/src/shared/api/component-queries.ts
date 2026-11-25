import { queryOptions } from '@tanstack/react-query'
import type { ApiClient } from './client'
import type { components, operations } from './generated/schema'
import type { OpenCodes } from './read-fields'
import {
  readAccessNotes,
  readMetricValue,
  readNullableNumber,
} from './read-fields'
import { readArray, readBoolean, readRecord, readString } from './response'

type Schema = components['schemas']

function readComponentOverview(
  value: unknown,
): OpenCodes<Schema['ComponentOverview']> {
  const raw = readRecord(value)
  const component = readRecord(raw.component)
  return {
    component: {
      id: readString(component.id),
      service_id: readString(component.service_id),
      repository_area_id: readString(component.repository_area_id),
      path_prefix: readString(component.path_prefix),
      status: readString(component.status),
      origin: readString(component.origin),
    },
    findings_by_state: readArray(raw.findings_by_state, (value) => {
      const row = readRecord(value)
      return {
        finding_kind: readString(row.finding_kind),
        state: readString(row.state),
        count: readMetricValue(row.count),
      }
    }),
    facts: readArray(raw.facts, (value) => {
      const fact = readRecord(value)
      const window = readRecord(fact.window)
      return {
        metric_id: readString(fact.metric_id),
        source: readString(fact.source),
        window: { from: readString(window.from), to: readString(window.to) },
        value: readMetricValue(fact.value),
      }
    }),
    hotspots: readArray(raw.hotspots, (value) => {
      const spot = readRecord(value)
      return {
        path: readString(spot.path),
        complexity: readNullableNumber(spot.complexity),
        churn: readNullableNumber(spot.churn),
        authors_share: readNullableNumber(spot.authors_share),
      }
    }),
    access_limited: readBoolean(raw.access_limited),
    access_notes: readAccessNotes(raw.access_notes),
    redactions: readArray(raw.redactions, readString),
  }
}

export function getComponentOverview(
  api: ApiClient,
  componentId: operations['getComponentOverview']['parameters']['path']['component_id'],
) {
  return queryOptions({
    queryKey: ['components', 'overview', componentId],
    queryFn: async ({ signal }) =>
      (
        await api.read(
          '/components/{component_id}/overview',
          readComponentOverview,
          {
            pathParams: { component_id: componentId },
            signal,
          },
        )
      ).data,
  })
}
