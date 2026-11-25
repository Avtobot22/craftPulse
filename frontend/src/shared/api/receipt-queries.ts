import { queryOptions } from '@tanstack/react-query'
import type { ApiClient } from './client'
import type { components, operations } from './generated/schema'
import { operationPolling } from './operation-polling'
import type { OpenCodes } from './read-fields'
import { readCursorPage, readNullableNumber, readNumber } from './read-fields'
import {
  readArray,
  readNullableString,
  readRecord,
  readString,
} from './response'

type Schema = components['schemas']
type ReceiptId = operations['getReceipt']['parameters']['path']['receipt_id']
export type ReceiptListParams = NonNullable<
  operations['listReceipts']['parameters']['query']
>
export type ReceiptRecordsParams = NonNullable<
  operations['listReceiptRecords']['parameters']['query']
>
export type IngestConflictListParams = NonNullable<
  operations['listIngestConflicts']['parameters']['query']
>

function readReceiptSummary(
  value: unknown,
): OpenCodes<Schema['ReceiptSummary']> {
  const raw = readRecord(value)
  return {
    id: readString(raw.id),
    channel: readString(raw.channel),
    status: readString(raw.status),
    received_at: readString(raw.received_at),
    schema_id: readNullableString(raw.schema_id),
    schema_version: readNullableString(raw.schema_version),
    record_count: readNumber(raw.record_count),
    accepted_count: readNumber(raw.accepted_count),
    rejected_count: readNumber(raw.rejected_count),
    reason_code: readNullableString(raw.reason_code),
    duplicate_of: readNullableString(raw.duplicate_of),
    affected_metrics: readArray(raw.affected_metrics, readString),
  }
}

function readReceipt(value: unknown): OpenCodes<Schema['Receipt']> {
  const raw = readRecord(value)
  return {
    ...readReceiptSummary(raw),
    replay_count: readNumber(raw.replay_count),
    conflict_count: readNumber(raw.conflict_count),
    retry_after_seconds: readNullableNumber(raw.retry_after_seconds),
    result_url: readNullableString(raw.result_url),
  }
}

function readReceiptRecord(value: unknown): OpenCodes<Schema['ReceiptRecord']> {
  const raw = readRecord(value)
  return {
    record_no: readNumber(raw.record_no),
    record_kind: readString(raw.record_kind),
    native_key: readNullableString(raw.native_key),
    outcome: readString(raw.outcome),
    reason_code: readNullableString(raw.reason_code),
  }
}

function readIngestConflict(
  value: unknown,
): OpenCodes<Schema['IngestConflict']> {
  const raw = readRecord(value)
  return {
    id: readString(raw.id),
    receipt_id: readString(raw.receipt_id),
    record_no: readNumber(raw.record_no),
    existing_receipt_id: readString(raw.existing_receipt_id),
    existing_record_no: readNumber(raw.existing_record_no),
    native_key: readString(raw.native_key),
    detected_at: readString(raw.detected_at),
    status: readString(raw.status),
    resolution: readNullableString(raw.resolution),
  }
}

export function listReceipts(api: ApiClient, params: ReceiptListParams = {}) {
  return queryOptions({
    queryKey: ['receipts', 'list', params],
    queryFn: async ({ signal }): Promise<OpenCodes<Schema['ReceiptPage']>> =>
      (
        await api.read(
          '/receipts',
          (value) => readCursorPage(value, readReceiptSummary),
          {
            query: params,
            signal,
          },
        )
      ).data,
  })
}

export function getReceipt(api: ApiClient, receiptId: ReceiptId) {
  return queryOptions({
    ...operationPolling('quarantined'),
    queryKey: ['receipts', 'detail', receiptId],
    queryFn: async ({ signal }) =>
      (
        await api.read('/receipts/{receipt_id}', readReceipt, {
          pathParams: { receipt_id: receiptId },
          signal,
        })
      ).data,
  })
}

export function listReceiptRecords(
  api: ApiClient,
  receiptId: ReceiptId,
  params: ReceiptRecordsParams = {},
) {
  return queryOptions({
    queryKey: ['receipts', 'records', receiptId, params],
    queryFn: async ({
      signal,
    }): Promise<OpenCodes<Schema['ReceiptRecordPage']>> =>
      (
        await api.read(
          '/receipts/{receipt_id}/records',
          (value) => readCursorPage(value, readReceiptRecord),
          {
            pathParams: { receipt_id: receiptId },
            query: params,
            signal,
          },
        )
      ).data,
  })
}

export function listIngestConflicts(
  api: ApiClient,
  params: IngestConflictListParams = {},
) {
  return queryOptions({
    queryKey: ['ingest-conflicts', 'list', params],
    queryFn: async ({
      signal,
    }): Promise<OpenCodes<Schema['IngestConflictPage']>> =>
      (
        await api.read(
          '/ingest-conflicts',
          (value) => readCursorPage(value, readIngestConflict),
          {
            query: params,
            signal,
          },
        )
      ).data,
  })
}
