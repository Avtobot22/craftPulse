import { QueryClient } from '@tanstack/react-query'
import { expect, it, vi } from 'vitest'
import {
  connectionExample,
  syncStatusExample,
} from '../../../test/admin-fixtures'
import { jsonResponse } from '../../../test/fixtures'
import {
  ApiClient,
  getConnection,
  getConnectionProbe,
  getReceipt,
  getSyncStatus,
  listAdapters,
  listConnections,
  listIngestConflicts,
  listPersons,
  listReceiptRecords,
  listReceipts,
  listSourceAccounts,
} from './index'

it('reads the connection by ID with its ETag and keeps connections in separate cache entries', async () => {
  const client = new QueryClient()
  const firstResponse = jsonResponse(connectionExample)
  firstResponse.headers.set('ETag', '"connection-1"')
  const fetcher = vi
    .fn<typeof fetch>()
    .mockResolvedValueOnce(firstResponse)
    .mockResolvedValueOnce(jsonResponse({ ...connectionExample, id: 'other' }))
  const api = new ApiClient(fetcher)
  const first = getConnection(api, connectionExample.id)
  const second = getConnection(api, 'other')
  try {
    await expect(client.query(first)).resolves.toEqual({
      data: connectionExample,
      etag: '"connection-1"',
    })
    await expect(client.query(second)).resolves.toMatchObject({
      data: { id: 'other' },
    })
    expect(client.getQueryData(first.queryKey)?.etag).toBe('"connection-1"')
  } finally {
    client.clear()
  }
})

it('preserves an unknown error code alongside the last success in the sync response', async () => {
  const firstKind = syncStatusExample.data_kinds[0]
  if (!firstKind)
    throw new Error('Expected one data kind in the OpenAPI example')
  const response = {
    ...syncStatusExample,
    data_kinds: [{ ...firstKind, last_error_code: 'provider_busy' }],
  }
  const api = new ApiClient(() => Promise.resolve(jsonResponse(response)))
  const client = new QueryClient()
  try {
    await expect(
      client.query(getSyncStatus(api, syncStatusExample.connection_id)),
    ).resolves.toEqual(response)
  } finally {
    client.clear()
  }
})

it('passes ids, filters and abort signals to the administration reads and rejects malformed data', async () => {
  const fetcher = vi
    .fn<typeof fetch>()
    .mockImplementation(() => Promise.resolve(jsonResponse({})))
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const api = new ApiClient(fetcher)
  const queries = [
    () => client.query(listAdapters(api)),
    () => client.query(listConnections(api, { status: 'draft' })),
    () => client.query(getConnection(api, 'c/1')),
    () => client.query(getSyncStatus(api, 'c/1')),
    () => client.query(getConnectionProbe(api, 'c/1', 'probe/1')),
    () => client.query(listReceipts(api, { connection_id: 'c/1' })),
    () => client.query(getReceipt(api, 'r/1')),
    () => client.query(listReceiptRecords(api, 'r/1', { outcome: 'rejected' })),
    () => client.query(listIngestConflicts(api, { status: 'resolved' })),
    () => client.query(listPersons(api, { kind: 'human' })),
    () => client.query(listSourceAccounts(api, { assignment: 'unassigned' })),
  ]
  try {
    for (const query of queries) {
      await expect(query()).rejects.toMatchObject({ kind: 'invalid-response' })
    }
    expect(fetcher.mock.calls.map(([url]) => url)).toEqual([
      '/api/v1/adapters',
      '/api/v1/connections?status=draft',
      '/api/v1/connections/c%2F1',
      '/api/v1/connections/c%2F1/sync-status',
      '/api/v1/connections/c%2F1/probes/probe%2F1',
      '/api/v1/receipts?connection_id=c%2F1',
      '/api/v1/receipts/r%2F1',
      '/api/v1/receipts/r%2F1/records?outcome=rejected',
      '/api/v1/ingest-conflicts?status=resolved',
      '/api/v1/persons?kind=human',
      '/api/v1/source-accounts?assignment=unassigned',
    ])
    expect(
      fetcher.mock.calls.every(
        ([, request]) => request?.signal instanceof AbortSignal,
      ),
    ).toBe(true)
  } finally {
    client.clear()
  }
})
