import { queryOptions } from '@tanstack/react-query'
import {
  allConnections,
  getPersons,
  getSourceAccounts,
  InvalidResponse,
} from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { providerLabel } from '@/shared/config'
import type { Account, RowResult } from '../model/assignments'

const directoryLimit = 100

async function* pages<T>(
  read: (
    cursor: string | undefined,
  ) => Promise<{ data: T[]; next_cursor: string | null }>,
) {
  const seen = new Set<string>()
  let cursor: string | undefined
  do {
    const page = await read(cursor)
    yield page.data
    cursor = page.next_cursor ?? undefined
    if (cursor !== undefined) {
      if (seen.has(cursor)) throw new InvalidResponse()
      seen.add(cursor)
    }
  } while (cursor !== undefined)
}

async function readAll<T>(directory: AsyncIterable<T[]>) {
  const items: T[] = []
  for await (const page of directory) items.push(...page)
  return items
}

export function peopleDirectory(api: ApiClient) {
  return queryOptions({
    queryKey: ['persons', 'account-directory'],
    queryFn: ({ signal }) =>
      readAll(
        pages((cursor) =>
          getPersons(
            api,
            { limit: directoryLimit, ...(cursor ? { cursor } : {}) },
            signal,
          ),
        ),
      ),
  })
}

export function instanceDirectory(api: ApiClient) {
  return queryOptions({
    ...allConnections(api),
    select: (connections) => {
      const instances = new Map<
        string,
        { name: string; connections: Set<string> }
      >()
      for (const { name, instance, provider } of connections) {
        const existing = instances.get(instance.id)
        if (existing) existing.connections.add(name)
        else
          instances.set(instance.id, {
            name:
              instance.base_url ??
              `${providerLabel(provider)} (${instance.id})`,
            connections: new Set([name]),
          })
      }
      return [...instances].map(([id, { name, connections }]) => ({
        id,
        label: `${name} · ${[...connections].join(', ')}`,
      }))
    },
  })
}

// A row can move off its filtered page after a write. The API only exposes
// list reads, so resolve the submitted IDs within each instance.
export async function readAffectedAccounts(
  api: ApiClient,
  rows: readonly RowResult[],
) {
  const idsByInstance = new Map<string, Set<string>>()
  for (const { account } of rows) {
    const ids = idsByInstance.get(account.instance_id) ?? new Set<string>()
    ids.add(account.id)
    idsByInstance.set(account.instance_id, ids)
  }

  const found = new Map<string, Account>()
  const signal = AbortSignal.timeout(30_000)
  for (const [instance_id, ids] of idsByInstance) {
    for await (const page of pages((cursor) =>
      getSourceAccounts(
        api,
        { instance_id, limit: directoryLimit, ...(cursor ? { cursor } : {}) },
        signal,
      ),
    )) {
      for (const account of page) {
        if (ids.delete(account.id)) found.set(account.id, account)
      }
      if (!ids.size) break
    }
  }
  return found
}
