import { infiniteQueryOptions, queryOptions } from '@tanstack/react-query'
import {
  InvalidResponse,
  readArray,
  readCursorPage,
  readMetricValue,
  readNullableNumber,
  readNullableString,
  readNumber,
  readRecord,
  readString,
} from '@/shared/api'
import type { ApiClient, components, OpenCodes } from '@/shared/api'
import type { ChangeAttempt } from '@/shared/ui'

type Schemas = components['schemas']
export type TargetDraft = Schemas['TargetDraft']
export type TargetVersion = OpenCodes<Schemas['TargetVersion']>
export type TargetGroup = Pick<TargetDraft, 'task_type' | 'size_label'>
export type TargetPeriod = NonNullable<TargetDraft['basis_period']>
export type BaselineGroup = OpenCodes<
  Schemas['TargetBaselineList']['data'][number]
>

const versionPath = '/teams/{team_id}/target-versions'

function readBaselines(value: unknown) {
  const raw = readRecord(value)
  return {
    revision_id: readString(raw.revision_id),
    data: readArray(raw.data, (item): BaselineGroup => {
      const row = readRecord(item)
      const p85 = readMetricValue(row.p85)
      return {
        task_type: readString(row.task_type),
        size_label: readNullableString(row.size_label),
        p85: {
          ...p85,
          low_sample: p85.low_sample || (p85.n !== null && p85.n < 20),
        },
      }
    }),
  }
}

function readTargetVersion(value: unknown): TargetVersion {
  const raw = readRecord(value)
  const period = raw.basis_period === null ? null : readRecord(raw.basis_period)
  const duration = readNumber(raw.duration_seconds)
  const version = readNumber(raw.version_no)
  if (
    !Number.isSafeInteger(duration) ||
    duration < 1 ||
    !Number.isSafeInteger(version) ||
    version < 1
  ) {
    throw new InvalidResponse()
  }
  return {
    task_type: readString(raw.task_type),
    size_label: readString(raw.size_label),
    version_no: version,
    base_version_no: readNullableNumber(raw.base_version_no),
    duration_seconds: duration,
    basis: readString(raw.basis),
    basis_revision_id: readNullableString(raw.basis_revision_id),
    basis_value_seconds: readNullableNumber(raw.basis_value_seconds),
    basis_n: readNullableNumber(raw.basis_n),
    basis_period: period && {
      from: readString(period.from),
      to: readString(period.to),
    },
    confirmed_by: readString(raw.confirmed_by),
    confirmed_at: readString(raw.confirmed_at),
  }
}

export function mappingQuery(api: ApiClient, teamId: string) {
  return queryOptions({
    queryKey: ['target-settings', teamId, 'mapping'],
    queryFn: async ({ signal }) => {
      const { data } = await api.read(
        '/teams/{team_id}/mapping-versions',
        (value) =>
          readCursorPage(value, (item) =>
            readNumber(readRecord(item).version_no),
          ),
        {
          pathParams: { team_id: teamId },
          query: { limit: 1 },
          signal,
        },
      )
      return data.data.length > 0
    },
  })
}

export function baselineQuery(
  api: ApiClient,
  teamId: string,
  period: TargetPeriod,
) {
  return queryOptions({
    queryKey: ['target-settings', teamId, 'baselines', period],
    queryFn: async ({ signal }) =>
      (
        await api.read('/teams/{team_id}/target-baselines', readBaselines, {
          pathParams: { team_id: teamId },
          query: period,
          signal,
        })
      ).data,
  })
}

export async function readVersionPage(
  api: ApiClient,
  teamId: string,
  group: TargetGroup,
  signal: AbortSignal,
  cursor?: string,
) {
  const result = await api.read(
    versionPath,
    (value) => readCursorPage(value, readTargetVersion),
    {
      pathParams: { team_id: teamId },
      query: { ...group, cursor },
      signal,
    },
  )
  if (
    result.data.data.some(
      (version) =>
        version.task_type !== group.task_type ||
        version.size_label !== group.size_label,
    ) ||
    result.etag === null ||
    !/^"\d+"$/u.test(result.etag) ||
    (!cursor &&
      result.etag !== `"${String(result.data.data[0]?.version_no ?? 0)}"`)
  ) {
    throw new InvalidResponse()
  }
  return { data: result.data, etag: result.etag }
}

export function versionQuery(
  api: ApiClient,
  teamId: string,
  group: TargetGroup,
) {
  return infiniteQueryOptions({
    queryKey: ['target-settings', teamId, 'versions', group],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ signal, pageParam }) =>
      readVersionPage(api, teamId, group, signal, pageParam),
    getNextPageParam: (last, _, __, pageParams) => {
      const cursor = last.data.next_cursor
      return cursor && !pageParams.includes(cursor) ? cursor : undefined
    },
  })
}

export async function createTargetVersion(
  api: ApiClient,
  teamId: string,
  attempt: ChangeAttempt<TargetDraft>,
  signal: AbortSignal,
) {
  const { data } = await api.update(
    'POST',
    versionPath,
    attempt.draft,
    readTargetVersion,
    {
      pathParams: { team_id: teamId },
      ifMatch: attempt.etag,
      idempotencyKey: attempt.idempotencyKey,
      signal,
    },
  )
  if (
    data.task_type !== attempt.draft.task_type ||
    data.size_label !== attempt.draft.size_label
  ) {
    throw new InvalidResponse()
  }
  return data
}
