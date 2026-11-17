import type { components, operations } from './generated/schema'
import { queryOptions } from '@tanstack/react-query'
import type { ApiClient } from './client'
import {
  readArray,
  readBoolean,
  readEnum,
  readNullableString,
  readRecord,
  readString,
} from './response'

type Schema = components['schemas']
export type Team = Omit<Schema['Team'], 'status'> & {
  status: Schema['Team']['status'] | 'unknown'
}
export type TeamPage = Omit<Schema['TeamPage'], 'data'> & { data: Team[] }
export type AccessNote = Omit<Schema['AccessNote'], 'code'> & {
  code: Schema['AccessNote']['code'] | 'unknown'
}
export type ProjectPage = Omit<
  Schema['ProjectPage'],
  'data' | 'access_notes'
> & {
  data: Team[]
  access_notes: AccessNote[]
}
export type ServicePage = Omit<Schema['ServicePage'], 'data'> & { data: Team[] }
export type TeamParams = NonNullable<
  operations['listTeams']['parameters']['query']
>
export type ProjectParams = NonNullable<
  operations['listProjects']['parameters']['query']
>
export type ServiceParams = NonNullable<
  operations['listServices']['parameters']['query']
>

export function readTeam(value: unknown): Team {
  const raw = readRecord(value)
  return {
    id: readString(raw.id),
    name: readString(raw.name),
    status: readEnum<Schema['Team']['status']>(raw.status, [
      'active',
      'archived',
    ]),
  }
}

export function readTeamPage(value: unknown): TeamPage {
  const raw = readRecord(value)
  return {
    data: readArray(raw.data, readTeam),
    next_cursor: readNullableString(raw.next_cursor),
  }
}

function readAccessNote(value: unknown): AccessNote {
  const raw = readRecord(value)
  return {
    code: readEnum<Schema['AccessNote']['code']>(raw.code, [
      'restricted_by_access',
      'access_check_unavailable',
      'source_access_lost',
    ]),
    ...(raw.connection_id === undefined
      ? {}
      : { connection_id: readNullableString(raw.connection_id) }),
  }
}

export function readProjectPage(value: unknown): ProjectPage {
  const raw = readRecord(value)
  return {
    ...readTeamPage(value),
    access_limited: readBoolean(raw.access_limited),
    access_notes: readArray(raw.access_notes, readAccessNote),
  }
}

export function listTeams(api: ApiClient, params: TeamParams = {}) {
  return queryOptions({
    queryKey: ['teams', params],
    queryFn: async ({ signal }) =>
      (await api.read('/teams', readTeamPage, { query: params, signal })).data,
  })
}
export function listProjects(api: ApiClient, params: ProjectParams = {}) {
  return queryOptions({
    queryKey: ['projects', params],
    queryFn: async ({ signal }) =>
      (await api.read('/projects', readProjectPage, { query: params, signal }))
        .data,
  })
}
export function listServices(api: ApiClient, params: ServiceParams = {}) {
  return queryOptions({
    queryKey: ['services', params],
    queryFn: async ({ signal }): Promise<ServicePage> =>
      (await api.read('/services', readTeamPage, { query: params, signal }))
        .data,
  })
}
