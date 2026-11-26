import { expect, it } from 'vitest'
import { adminLinks } from './admin-routes'

it('links sync status to the receipt journal with a connection filter', () => {
  expect(adminLinks.receiptJournal({ connection_id: 'instance/1' })).toBe(
    '/settings/receipts?connection_id=instance%2F1',
  )
})

it('links a receipt to repeat import with both schema parameters', () => {
  expect(
    adminLinks.importPackage({ schema_id: 'tasks', schema_version: '1+2' }),
  ).toBe('/settings/import?schema_id=tasks&schema_version=1%2B2')
})

it('links to the global conflict list by status', () => {
  expect(adminLinks.ingestConflicts({ status: 'resolved' })).toBe(
    '/settings/ingest-conflicts?status=resolved',
  )
  expect(adminLinks.ingestConflicts()).toBe('/settings/ingest-conflicts')
})

it('only copies schema fields to the repeat import URL', () => {
  const receipt = {
    schema_id: 'tasks',
    schema_version: '1',
    reason_code: 'schema_unsupported',
  }
  expect(adminLinks.importPackage(receipt)).toBe(
    '/settings/import?schema_id=tasks&schema_version=1',
  )
})
