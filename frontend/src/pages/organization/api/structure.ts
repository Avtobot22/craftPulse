import {
  InvalidResponse,
  readArray,
  readCursorPage,
  readNullableString,
  readNumber,
  readRecord,
  readString,
} from '@/shared/api'
import type { ApiClient, components } from '@/shared/api'

type Schema = components['schemas']
export type Structure = Schema['OrganizationStructure']
export type StructureInput = Schema['OrganizationStructureInput']

function readNamed(value: unknown): Structure['teams'][number] {
  const raw = readRecord(value)
  if (raw.status !== 'active' && raw.status !== 'archived') {
    throw new InvalidResponse()
  }
  return {
    id: readString(raw.id),
    name: readString(raw.name),
    status: raw.status,
  }
}

function readComponent(value: unknown): Schema['Component'] {
  const raw = readRecord(value)
  if (
    (raw.status !== 'confirmed' &&
      raw.status !== 'proposed' &&
      raw.status !== 'rejected') ||
    (raw.origin !== 'manual' && raw.origin !== 'manifest')
  )
    throw new InvalidResponse()
  return {
    id: readString(raw.id),
    service_id: readString(raw.service_id),
    repository_area_id: readString(raw.repository_area_id),
    path_prefix: readString(raw.path_prefix),
    status: raw.status,
    origin: raw.origin,
  }
}

function readInterval(raw: Record<string, unknown>) {
  return {
    valid_from: readString(raw.valid_from),
    valid_to: readNullableString(raw.valid_to),
  }
}

function readStructure(value: unknown): Structure {
  const raw = readRecord(value)
  const version = readNumber(raw.version)
  if (!Number.isSafeInteger(version) || version < 0) throw new InvalidResponse()
  return {
    version,
    teams: readArray(raw.teams, readNamed),
    projects: readArray(raw.projects, readNamed),
    services: readArray(raw.services, readNamed),
    components: readArray(raw.components, readComponent),
    team_projects: readArray(raw.team_projects, (value) => {
      const link = readRecord(value)
      return {
        team_id: readString(link.team_id),
        project_id: readString(link.project_id),
        ...readInterval(link),
      }
    }),
    team_members: readArray(raw.team_members, (value) => {
      const link = readRecord(value)
      return {
        team_id: readString(link.team_id),
        person_id: readString(link.person_id),
        ...readInterval(link),
      }
    }),
    project_areas: readArray(raw.project_areas, (value) => {
      const link = readRecord(value)
      return {
        project_id: readString(link.project_id),
        area_id: readString(link.area_id),
        ...readInterval(link),
      }
    }),
  }
}

export function getStructure(api: ApiClient, signal: AbortSignal) {
  return api.read('/organization/structure', readStructure, { signal })
}

export function replaceStructure(
  api: ApiClient,
  body: StructureInput,
  ifMatch: string,
  signal: AbortSignal,
) {
  return api.update('PUT', '/organization/structure', body, readStructure, {
    signal,
    ifMatch,
  })
}

export interface Choice {
  id: string
  name: string
}

export async function collectChoices<T extends Choice>(
  read: (cursor?: string) => Promise<{ data: T[]; next_cursor: string | null }>,
) {
  const choices = new Map<string, T>()
  const seen = new Set<string>()
  let cursor: string | undefined
  do {
    const page = await read(cursor)
    for (const item of page.data) choices.set(item.id, item)
    cursor = page.next_cursor ?? undefined
    if (cursor) {
      if (seen.has(cursor)) throw new InvalidResponse()
      seen.add(cursor)
    }
  } while (cursor)
  return [...choices.values()]
}

export function getAreas(api: ApiClient, signal: AbortSignal) {
  return collectChoices(
    async (cursor) =>
      (
        await api.read(
          '/areas',
          (value) =>
            readCursorPage(value, (item) => {
              const raw = readRecord(item)
              return {
                id: readString(raw.id),
                name: readString(raw.display_name),
                kind: readString(raw.kind),
              }
            }),
          { signal, query: { cursor } },
        )
      ).data,
  )
}
