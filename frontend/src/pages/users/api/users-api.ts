import { queryOptions } from '@tanstack/react-query'
import {
  ApiError,
  InvalidResponse,
  readArray,
  readCursorPage,
  readEnum,
  readNullableString,
  readRecord,
  readProjectPage,
  readTeamPage,
  readString,
} from '@/shared/api'
import type { ApiClient, components, operations, Team } from '@/shared/api'

type Schemas = components['schemas']
export type User = Omit<Schemas['User'], 'status'> & {
  status: Schemas['User']['status'] | 'unknown'
}
export type UserParams = NonNullable<
  operations['listUsers']['parameters']['query']
>
export type RoleInput = Omit<Schemas['RoleAssignmentInput'], 'role'> & {
  role: string
}
export type Assignment = RoleInput &
  Pick<Schemas['RoleAssignment'], 'assigned_at'>
export const usersKey = ['organization-users'] as const

function readUser(value: unknown): User {
  const raw = readRecord(value)
  return {
    id: readString(raw.id),
    display_name: readNullableString(raw.display_name),
    status: readEnum(raw.status, ['active', 'blocked']),
    person_id: readNullableString(raw.person_id),
  }
}

function readRoles(value: unknown) {
  return readArray(readRecord(value).data, (item): Assignment => {
    const raw = readRecord(item)
    return {
      role: readString(raw.role),
      team_id: readNullableString(raw.team_id),
      project_id: readNullableString(raw.project_id),
      service_id: readNullableString(raw.service_id),
      assigned_at: readString(raw.assigned_at),
    }
  })
}

function fetchUsers(api: ApiClient, query: UserParams, signal: AbortSignal) {
  return api.read('/users', (value) => readCursorPage(value, readUser), {
    query,
    signal,
  })
}

export function usersPage(api: ApiClient, params: UserParams) {
  return queryOptions({
    queryKey: [...usersKey, 'page', params],
    queryFn: async ({ signal }) => (await fetchUsers(api, params, signal)).data,
  })
}

export function userDetails(api: ApiClient, userId: string) {
  return queryOptions({
    queryKey: [...usersKey, 'user', userId],
    queryFn: async ({ signal }) => {
      // listUserRoles does not carry the user profile and there is no GET /users/{id}.
      for await (const page of cursorPages(
        async (cursor) =>
          (
            await fetchUsers(
              api,
              {
                limit: 100,
                sort: 'display_name',
                ...(cursor ? { cursor } : {}),
              },
              signal,
            )
          ).data,
      )) {
        const user = page.find(({ id }) => id === userId)
        if (user) return user
      }
      throw new ApiError('not-found')
    },
  })
}

export function fetchRoles(
  api: ApiClient,
  userId: string,
  signal: AbortSignal,
) {
  return api.read('/users/{user_id}/roles', readRoles, {
    pathParams: { user_id: userId },
    signal,
  })
}

export function putRoles(
  api: ApiClient,
  userId: string,
  data: RoleInput[],
  ifMatch: string,
  signal: AbortSignal,
) {
  return api.replaceUserRoles(userId, { data }, readRoles, { ifMatch, signal })
}

export function userRoles(api: ApiClient, userId: string) {
  return queryOptions({
    queryKey: ['user-roles', userId],
    staleTime: Infinity,
    refetchOnMount: 'always',
    queryFn: ({ signal }) => fetchRoles(api, userId, signal),
  })
}

const areaLists = {
  team_id: '/teams',
  project_id: '/projects',
  service_id: '/services',
} as const
export type AreaField = keyof typeof areaLists

async function* cursorPages<T>(
  load: (cursor?: string) => Promise<{ data: T[]; next_cursor: string | null }>,
) {
  const seen = new Set<string>()
  let cursor: string | undefined
  do {
    const page = await load(cursor)
    yield page.data
    cursor = page.next_cursor ?? undefined
    if (cursor && seen.has(cursor)) throw new InvalidResponse()
    if (cursor) seen.add(cursor)
  } while (cursor)
}

export function roleAreas(api: ApiClient, field: AreaField) {
  return queryOptions({
    queryKey: ['role-areas', field],
    staleTime: 60_000,
    queryFn: async ({ signal }) => {
      const areas = new Map<string, Team>()
      const readPage = field === 'project_id' ? readProjectPage : readTeamPage
      for await (const page of cursorPages(
        async (cursor) =>
          (
            await api.read(areaLists[field], readPage, {
              signal,
              query: { cursor },
            })
          ).data,
      )) {
        for (const area of page) areas.set(area.id, area)
      }
      return areas
    },
  })
}
