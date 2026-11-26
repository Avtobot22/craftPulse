import type { components, operations } from '@/shared/api'
import { routePaths } from './routes'

export type ReceiptJournalSearch = Pick<
  NonNullable<operations['listReceipts']['parameters']['query']>,
  'connection_id'
>
export interface ImportPackageSearch {
  schema_id?: NonNullable<components['schemas']['Receipt']['schema_id']>
  schema_version?: NonNullable<
    components['schemas']['Receipt']['schema_version']
  >
}
export type IngestConflictsSearch = Pick<
  NonNullable<operations['listIngestConflicts']['parameters']['query']>,
  'status'
>

function withSearch(path: string, params: Record<string, string | undefined>) {
  const search = new URLSearchParams()
  for (const [name, value] of Object.entries(params)) {
    if (value !== undefined) search.set(name, value)
  }
  return search.size ? path + '?' + search.toString() : path
}

export const adminLinks = {
  // UI-11 → UI-13: filter the journal by the selected connection.
  receiptJournal: ({ connection_id }: ReceiptJournalSearch = {}) =>
    withSearch(routePaths.receiptJournal, { connection_id }),
  // UI-13 → UI-12: prefill the schema for a repeat import.
  importPackage: ({ schema_id, schema_version }: ImportPackageSearch = {}) =>
    withSearch(routePaths.importPackage, { schema_id, schema_version }),
  // Unset status uses the API default (open).
  ingestConflicts: ({ status }: IngestConflictsSearch = {}) =>
    withSearch(routePaths.ingestConflicts, { status }),
}
