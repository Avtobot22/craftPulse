import { queryOptions } from '@tanstack/react-query'
import type { ApiClient } from './client'
import type { components, operations } from './generated/schema'
import { operationPolling } from './operation-polling'
import type { OpenCodes } from './read-fields'
import { readCursorPage, readNullableNumber, readNumber } from './read-fields'
import {
  readArray,
  readBoolean,
  readNullableString,
  readRecord,
  readString,
} from './response'

type Schema = components['schemas']
type ConnectionId =
  operations['getConnection']['parameters']['path']['connection_id']
type ProbePath = operations['getConnectionProbe']['parameters']['path']
export type ConnectionListParams = NonNullable<
  operations['listConnections']['parameters']['query']
>

function readCapability(value: unknown): OpenCodes<Schema['Capability']> {
  const raw = readRecord(value)
  return {
    capability: readString(raw.capability),
    state: readString(raw.state),
    reason_code: readNullableString(raw.reason_code),
    checked_at: readString(raw.checked_at),
  }
}

function readAdapters(value: unknown): OpenCodes<Schema['AdapterList']> {
  const raw = readRecord(value)
  return {
    data: readArray(raw.data, (item) => {
      const adapter = readRecord(item)
      return {
        provider: readString(adapter.provider),
        display_name: readString(adapter.display_name),
        editions:
          adapter.editions === null
            ? []
            : readArray(adapter.editions, readString),
        contract_versions: readArray(adapter.contract_versions, readString),
        data_kinds: readArray(adapter.data_kinds, readString),
        scope_kinds: readArray(adapter.scope_kinds, readString),
        credential_fields: readArray(adapter.credential_fields, (item) => {
          const field = readRecord(item)
          return {
            name: readString(field.name),
            label: readString(field.label),
            required: readBoolean(field.required),
            secret: readBoolean(field.secret),
          }
        }),
      }
    }),
  }
}

function readConnectionScope(
  value: unknown,
): OpenCodes<Schema['ConnectionScope']> {
  const raw = readRecord(value)
  return {
    id: readString(raw.id),
    kind: readString(raw.kind),
    native_scope_id: readString(raw.native_scope_id),
    display_name: readString(raw.display_name),
    area_id: readNullableString(raw.area_id),
    history_from: readNullableString(raw.history_from),
    ...(raw.repository_scope_id === undefined
      ? {}
      : { repository_scope_id: readNullableString(raw.repository_scope_id) }),
  }
}

export function readConnection(
  value: unknown,
): OpenCodes<Schema['Connection']> {
  const raw = readRecord(value)
  const instance = readRecord(raw.instance)
  return {
    id: readString(raw.id),
    name: readString(raw.name),
    provider: readString(raw.provider),
    instance: {
      id: readString(instance.id),
      base_url: readNullableString(instance.base_url),
      edition: readNullableString(instance.edition),
      source_version: readNullableString(instance.source_version),
    },
    status: readString(raw.status),
    adapter_kind: readString(raw.adapter_kind),
    access_mode: readString(raw.access_mode),
    credentials_set_at: readNullableString(raw.credentials_set_at),
    sync_paused: readBoolean(raw.sync_paused),
    paused_until: readNullableString(raw.paused_until),
    last_success_at: readNullableString(raw.last_success_at),
    scopes: readArray(raw.scopes, readConnectionScope),
    capabilities: readArray(raw.capabilities, readCapability),
    ...(raw.webhook_url === undefined
      ? {}
      : { webhook_url: readNullableString(raw.webhook_url) }),
  }
}

function readProbe(value: unknown): OpenCodes<Schema['ConnectionProbe']> {
  const raw = readRecord(value)
  return {
    id: readString(raw.id),
    status: readString(raw.status),
    started_at: readString(raw.started_at),
    finished_at: readNullableString(raw.finished_at),
    edition: readNullableString(raw.edition),
    source_version: readNullableString(raw.source_version),
    contract_version: readNullableString(raw.contract_version),
    capabilities: readArray(raw.capabilities, readCapability),
    reason_code: readNullableString(raw.reason_code),
    retry_after_seconds: readNullableNumber(raw.retry_after_seconds),
    configuration_version: readNumber(raw.configuration_version),
  }
}

function readSyncDataKind(
  value: unknown,
): OpenCodes<Schema['SyncDataKindStatus']> {
  const raw = readRecord(value)
  return {
    data_kind: readString(raw.data_kind),
    phase: readString(raw.phase),
    last_attempt_at: readNullableString(raw.last_attempt_at),
    last_outcome: readNullableString(raw.last_outcome),
    last_error_code: readNullableString(raw.last_error_code),
    failed_page_present: readBoolean(raw.failed_page_present),
    last_success_at: readNullableString(raw.last_success_at),
    watermark: readNullableString(raw.watermark),
    lag_seconds: readNullableNumber(raw.lag_seconds),
    history_from: readNullableString(raw.history_from),
    completeness: readString(raw.completeness),
    unknown_fields: readArray(raw.unknown_fields, readString),
    affected_metrics: readArray(raw.affected_metrics, readString),
  }
}

function readReconcile(
  value: unknown,
): OpenCodes<Schema['SyncStatus']['last_reconcile']> {
  if (value === null) return null
  const raw = readRecord(value)
  return {
    started_at: readString(raw.started_at),
    finished_at: readNullableString(raw.finished_at),
    outcome: readString(raw.outcome),
    expected_count: readNullableNumber(raw.expected_count),
    observed_count: readNullableNumber(raw.observed_count),
    missing_count: readNullableNumber(raw.missing_count),
    changed_count: readNullableNumber(raw.changed_count),
  }
}

function readWebhooks(
  value: unknown,
): OpenCodes<NonNullable<Schema['SyncStatus']['webhooks']>> | null {
  if (value === null) return null
  const raw = readRecord(value)
  return {
    last_received_at: readNullableString(raw.last_received_at),
    rejected_count: readNumber(raw.rejected_count),
    last_rejected_at: readNullableString(raw.last_rejected_at),
    last_reject_reason: readNullableString(raw.last_reject_reason),
  }
}

function readSyncStatus(value: unknown): OpenCodes<Schema['SyncStatus']> {
  const raw = readRecord(value)
  return {
    connection_id: readString(raw.connection_id),
    connection_status: readString(raw.connection_status),
    paused_until: readNullableString(raw.paused_until),
    data_kinds: readArray(raw.data_kinds, readSyncDataKind),
    last_reconcile: readReconcile(raw.last_reconcile),
    ...(raw.webhooks === undefined
      ? {}
      : { webhooks: readWebhooks(raw.webhooks) }),
  }
}

export function listAdapters(api: ApiClient) {
  return queryOptions({
    queryKey: ['adapters', 'list'],
    queryFn: async ({ signal }) =>
      (await api.read('/adapters', readAdapters, { signal })).data,
  })
}

export function listConnections(
  api: ApiClient,
  params: ConnectionListParams = {},
) {
  return queryOptions({
    queryKey: ['connections', 'list', params],
    queryFn: async ({ signal }): Promise<OpenCodes<Schema['ConnectionPage']>> =>
      (
        await api.read(
          '/connections',
          (value) => readCursorPage(value, readConnection),
          {
            query: params,
            signal,
          },
        )
      ).data,
  })
}

export function getConnection(api: ApiClient, connectionId: ConnectionId) {
  return queryOptions({
    queryKey: ['connections', 'detail', connectionId],
    queryFn: ({ signal }) =>
      api.read('/connections/{connection_id}', readConnection, {
        pathParams: { connection_id: connectionId },
        signal,
      }),
  })
}

export function getSyncStatus(api: ApiClient, connectionId: ConnectionId) {
  return queryOptions({
    queryKey: ['connections', 'sync-status', connectionId],
    queryFn: async ({ signal }) =>
      (
        await api.read(
          '/connections/{connection_id}/sync-status',
          readSyncStatus,
          {
            pathParams: { connection_id: connectionId },
            signal,
          },
        )
      ).data,
  })
}

export function getConnectionProbe(
  api: ApiClient,
  connectionId: ProbePath['connection_id'],
  probeId: ProbePath['probe_id'],
) {
  return queryOptions({
    ...operationPolling('pending', 'running'),
    queryKey: ['connections', 'probe', connectionId, probeId],
    queryFn: async ({ signal }) =>
      (
        await api.read(
          '/connections/{connection_id}/probes/{probe_id}',
          readProbe,
          {
            pathParams: { connection_id: connectionId, probe_id: probeId },
            signal,
          },
        )
      ).data,
  })
}
