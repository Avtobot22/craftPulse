import { queryOptions } from '@tanstack/react-query'
import {
  readArray,
  readEnum,
  readNullableString,
  readRecord,
  readString,
} from '@/shared/api'
import type { ApiClient, components } from '@/shared/api'

type Protocol = components['schemas']['InstallationProtocol']
type Check = Protocol['checks'][number]

function readCheck(value: unknown) {
  const raw = readRecord(value)
  return {
    check_kind: readEnum<Check['check_kind']>(raw.check_kind, [
      'site',
      'adapter',
      'adapter_store',
      'identity_provider',
      'update_source',
      'database',
      'object_store',
      'runner_network',
    ]),
    outcome: readEnum<Check['outcome']>(raw.outcome, [
      'passed',
      'failed',
      'skipped',
    ]),
    reason_code: readNullableString(raw.reason_code),
  }
}

function readProtocol(value: unknown) {
  const raw = readRecord(value)
  return {
    release_sha: readString(raw.release_sha),
    started_at: readString(raw.started_at),
    finished_at: readNullableString(raw.finished_at),
    outcome: readEnum<Protocol['outcome']>(raw.outcome, [
      'running',
      'passed',
      'failed',
    ]),
    checks: readArray(raw.checks, readCheck),
  }
}

export type InstallationProtocol = ReturnType<typeof readProtocol>

export function getLatestInstallationProtocol(
  api: ApiClient,
  organizationId: string,
) {
  return queryOptions({
    queryKey: ['latest-installation-protocol', organizationId],
    queryFn: async ({ signal }) =>
      (
        await api.read('/installation/protocols/latest', readProtocol, {
          signal,
        })
      ).data,
  })
}
