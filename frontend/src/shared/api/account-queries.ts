import { queryOptions } from '@tanstack/react-query'
import type { ApiClient } from './client'
import type { components, operations } from './generated/schema'
import type { OpenCodes } from './read-fields'
import { readCursorPage } from './read-fields'
import {
  readArray,
  readNullableString,
  readRecord,
  readString,
} from './response'

type Schema = components['schemas']
export type PersonListParams = NonNullable<
  operations['listPersons']['parameters']['query']
>
export type SourceAccountListParams = NonNullable<
  operations['listSourceAccounts']['parameters']['query']
>

function readPerson(value: unknown): OpenCodes<Schema['Person']> {
  const raw = readRecord(value)
  return {
    id: readString(raw.id),
    display_name: readNullableString(raw.display_name),
    kind: readString(raw.kind),
  }
}

function readSourceAccount(value: unknown): OpenCodes<Schema['SourceAccount']> {
  const raw = readRecord(value)
  const assignment = raw.assignment === null ? null : readRecord(raw.assignment)
  return {
    id: readString(raw.id),
    instance_id: readString(raw.instance_id),
    provider: readString(raw.provider),
    native_account_id: readString(raw.native_account_id),
    login: readNullableString(raw.login),
    display_name: readNullableString(raw.display_name),
    proposed_kind: readString(raw.proposed_kind),
    assignment: assignment && {
      id: readString(assignment.id),
      person_id: readNullableString(assignment.person_id),
      actor_kind: readString(assignment.actor_kind),
      confirmed_at: readString(assignment.confirmed_at),
    },
    proposals: readArray(raw.proposals, (item) => {
      const proposal = readRecord(item)
      return {
        person_id: readString(proposal.person_id),
        basis: readString(proposal.basis),
      }
    }),
  }
}

export async function getPersons(
  api: ApiClient,
  params: PersonListParams,
  signal: AbortSignal,
): Promise<OpenCodes<Schema['PersonPage']>> {
  return (
    await api.read('/persons', (value) => readCursorPage(value, readPerson), {
      query: params,
      signal,
    })
  ).data
}

export function listPersons(api: ApiClient, params: PersonListParams = {}) {
  return queryOptions({
    queryKey: ['persons', 'list', params],
    queryFn: ({ signal }) => getPersons(api, params, signal),
  })
}

export function listSourceAccounts(
  api: ApiClient,
  params: SourceAccountListParams = {},
) {
  return queryOptions({
    queryKey: ['source-accounts', 'list', params],
    queryFn: async ({
      signal,
    }): Promise<OpenCodes<Schema['SourceAccountPage']>> =>
      (
        await api.read(
          '/source-accounts',
          (value) => readCursorPage(value, readSourceAccount),
          {
            query: params,
            signal,
          },
        )
      ).data,
  })
}
