import {
  InvalidResponse,
  isDateTime,
  readArray,
  readEnum,
  readNullableString,
  readRecord,
  readReport,
  readString,
} from '@/shared/api'
import type { ApiClient, components } from '@/shared/api'

type Schemas = components['schemas']
export type SavedViewInput = Schemas['SavedViewInput']
type SavedFilters = Schemas['SavedViewFilters']

function readFilters(value: unknown): SavedFilters {
  const filters = readRecord(value)
  const step = readEnum(filters.step ?? '', ['week', 'month'])
  const group = readEnum(filters.group_by ?? '', ['task_type', 'size_label'])
  return {
    ...(step === 'unknown' ? {} : { step }),
    ...(group === 'unknown' ? {} : { group_by: group }),
    ...(filters.project_id === undefined
      ? {}
      : { project_id: readArray(filters.project_id, readString) }),
    ...(filters.task_type === undefined
      ? {}
      : { task_type: readString(filters.task_type) }),
    ...(filters.size_label === undefined
      ? {}
      : { size_label: readString(filters.size_label) }),
  }
}

function readSavedView(value: unknown) {
  const raw = readRecord(value)
  const createdAt = readString(raw.created_at)
  if (!isDateTime(createdAt)) throw new InvalidResponse()
  return {
    id: readString(raw.id),
    name: readString(raw.name),
    series_id: readString(raw.series_id),
    pinned_revision_id: readNullableString(raw.pinned_revision_id),
    created_at: createdAt,
    filters: readFilters(raw.filters),
  }
}

export const savedViewsKey = ['saved-views'] as const
export const savedViewKey = (id: string) => ['saved-view', id] as const

export async function listSavedViews(
  api: ApiClient,
  cursor: string | undefined,
  signal: AbortSignal,
) {
  const { data } = await api.read(
    '/saved-views',
    (value) => {
      const raw = readRecord(value)
      return {
        data: readArray(raw.data, readSavedView),
        next_cursor: readNullableString(raw.next_cursor),
      }
    },
    { query: { cursor, limit: 20 }, signal },
  )
  return data
}

export async function getSavedView(
  api: ApiClient,
  id: string,
  signal: AbortSignal,
) {
  const { data } = await api.read(
    '/saved-views/{saved_view_id}',
    (value) => {
      const raw = readRecord(value)
      const view = readSavedView(raw)
      const report = readReport(raw.report)
      if (
        view.id !== id ||
        report.revision.series_id !== view.series_id ||
        (view.pinned_revision_id !== null &&
          report.revision.id !== view.pinned_revision_id) ||
        (report.step !== 'week' && report.step !== 'month') ||
        (view.filters.step !== undefined && report.step !== view.filters.step)
      )
        throw new InvalidResponse()
      return { ...view, report, step: report.step }
    },
    { pathParams: { saved_view_id: id }, signal },
  )
  return data
}

export async function createSavedView(api: ApiClient, input: SavedViewInput) {
  const { data } = await api.createSavedView(input, readSavedView)
  return data
}

export type SavedView = ReturnType<typeof readSavedView>
export type OpenedSavedView = Awaited<ReturnType<typeof getSavedView>>
