import { readCursorPage } from '@/shared/api'
import type { ApiClient, ApiResponse } from '@/shared/api'
import type { ChangeAttempt } from '@/shared/ui'
import {
  readCalendarVersion,
  readCatalog,
  readMappingPreview,
  readMappingVersion,
} from './workflow-responses'
import type { CalendarDraft, MappingDraft } from './workflow-responses'

export interface VersionPage<Version> {
  data: Version[]
  next_cursor: string | null
}

export interface VersionApi<Draft, Version> {
  list: (
    signal: AbortSignal,
    cursor?: string,
  ) => Promise<ApiResponse<VersionPage<Version>>>
  create: (
    attempt: ChangeAttempt<Draft>,
    signal: AbortSignal,
  ) => Promise<ApiResponse<Version>>
}

export function workflowApi(api: ApiClient, teamId: string) {
  const pathParams = { team_id: teamId }

  const mapping: VersionApi<
    MappingDraft,
    ReturnType<typeof readMappingVersion>
  > = {
    list: (signal, cursor) =>
      api.read(
        '/teams/{team_id}/mapping-versions',
        (value) => readCursorPage(value, readMappingVersion),
        { pathParams, signal, query: { cursor } },
      ),
    create: (attempt, signal) =>
      api.update(
        'POST',
        '/teams/{team_id}/mapping-versions',
        attempt.draft,
        readMappingVersion,
        {
          pathParams,
          signal,
          ifMatch: attempt.etag,
          idempotencyKey: attempt.idempotencyKey,
        },
      ),
  }
  const calendar: VersionApi<
    CalendarDraft,
    ReturnType<typeof readCalendarVersion>
  > = {
    list: (signal, cursor) =>
      api.read(
        '/teams/{team_id}/calendar-versions',
        (value) => readCursorPage(value, readCalendarVersion),
        { pathParams, signal, query: { cursor } },
      ),
    create: (attempt, signal) =>
      api.update(
        'POST',
        '/teams/{team_id}/calendar-versions',
        attempt.draft,
        readCalendarVersion,
        {
          pathParams,
          signal,
          ifMatch: attempt.etag,
          idempotencyKey: attempt.idempotencyKey,
        },
      ),
  }

  return {
    mapping,
    calendar,
    async load(signal: AbortSignal) {
      const [catalog, mappingPage, calendarPage] = await Promise.all([
        api.read('/teams/{team_id}/workflow-catalog', readCatalog, {
          pathParams,
          signal,
        }),
        mapping.list(signal),
        calendar.list(signal),
      ])
      return { catalog: catalog.data, mappingPage, calendarPage }
    },
    async preview(draft: MappingDraft, signal: AbortSignal) {
      return (
        await api.previewMapping(teamId, draft, readMappingPreview, signal)
      ).data
    },
  }
}

export type WorkflowApi = ReturnType<typeof workflowApi>
export type WorkflowData = Awaited<ReturnType<WorkflowApi['load']>>
