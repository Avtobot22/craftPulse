import { queryOptions } from '@tanstack/react-query'
import type { ApiClient } from './client'
import type { components, operations } from './generated/schema'
import type { OpenCodes } from './read-fields'
import { readNullableNumber, readNumber, readStringMap } from './read-fields'
import {
  InvalidResponse,
  readArray,
  readBoolean,
  readNullableString,
  readRecord,
  readString,
} from './response'

type Schema = components['schemas']
export type CiConfigurationParams =
  operations['getCiConfiguration']['parameters']['query']
export type UploadTokenListParams =
  operations['listUploadTokens']['parameters']['query']

function readCheckProfile(
  value: unknown,
): OpenCodes<Schema['CheckProfileVersion']> {
  const raw = readRecord(value)
  return {
    version_no: readNumber(raw.version_no),
    base_version_no: readNullableNumber(raw.base_version_no),
    entries: readArray(raw.entries, (value) => {
      const entry = readRecord(value)
      return {
        component_id: readString(entry.component_id),
        language: readString(entry.language),
        analysis_kind: readString(entry.analysis_kind),
        mode: readString(entry.mode),
        profile_id: readString(entry.profile_id),
        tool_versions: readStringMap(entry.tool_versions),
      }
    }),
    uncovered: readArray(raw.uncovered, (value) => {
      const entry = readRecord(value)
      return {
        component_id: readString(entry.component_id),
        language: readString(entry.language),
        reason_code: readString(entry.reason_code),
      }
    }),
    confirmed_by: readString(raw.confirmed_by),
    confirmed_at: readString(raw.confirmed_at),
  }
}

function readCiConfiguration(
  value: unknown,
): OpenCodes<Schema['CiConfiguration']> {
  const raw = readRecord(value)
  return {
    ci_system: readString(raw.ci_system),
    profile_version_no: readNumber(raw.profile_version_no),
    snippet: readString(raw.snippet),
    variables: readArray(raw.variables, (value) => {
      const variable = readRecord(value)
      return {
        name: readString(variable.name),
        masked: readBoolean(variable.masked),
      }
    }),
  }
}

function readUploadTokenPage(value: unknown): Schema['UploadTokenPage'] {
  const raw = readRecord(value)
  return {
    data: readArray(raw.data, (value) => {
      const token = readRecord(value)
      const slot = token.slot
      if (slot !== 1 && slot !== 2) throw new InvalidResponse()
      return {
        id: readString(token.id),
        project_id: readString(token.project_id),
        slot,
        created_at: readString(token.created_at),
        revoked_at: readNullableString(token.revoked_at),
      }
    }),
    next_cursor: readNullableString(raw.next_cursor),
  }
}

function readBaselines(value: unknown): OpenCodes<Schema['BaselineList']> {
  const raw = readRecord(value)
  return {
    data: readArray(raw.data, (value) => {
      const row = readRecord(value)
      return {
        component_id: readString(row.component_id),
        language: readString(row.language),
        analysis_kind: readString(row.analysis_kind),
        state: readString(row.state),
        report_id: readNullableString(row.report_id),
        branch: readString(row.branch),
        sha: readNullableString(row.sha),
        established_at: readNullableString(row.established_at),
        reason_code: readNullableString(row.reason_code),
      }
    }),
  }
}

export function getCheckProfile(
  api: ApiClient,
  projectId: operations['getCheckProfile']['parameters']['path']['project_id'],
) {
  return queryOptions({
    queryKey: ['check-profiles', 'current', projectId],
    queryFn: ({ signal }) =>
      api.read('/projects/{project_id}/check-profiles', readCheckProfile, {
        pathParams: { project_id: projectId },
        signal,
      }),
  })
}

export function getCiConfiguration(
  api: ApiClient,
  projectId: operations['getCiConfiguration']['parameters']['path']['project_id'],
  params: CiConfigurationParams,
) {
  return queryOptions({
    queryKey: ['check-profiles', 'ci-configuration', projectId, params],
    queryFn: async ({ signal }) =>
      (
        await api.read(
          '/projects/{project_id}/ci-configuration',
          readCiConfiguration,
          {
            pathParams: { project_id: projectId },
            query: params,
            signal,
          },
        )
      ).data,
  })
}

export function listUploadTokens(
  api: ApiClient,
  params: UploadTokenListParams,
) {
  return queryOptions({
    queryKey: ['upload-tokens', 'list', params],
    queryFn: async ({ signal }) =>
      (
        await api.read('/upload-tokens', readUploadTokenPage, {
          query: params,
          signal,
        })
      ).data,
  })
}

export function listBaselines(
  api: ApiClient,
  projectId: operations['listBaselines']['parameters']['path']['project_id'],
) {
  return queryOptions({
    queryKey: ['baselines', 'list', projectId],
    queryFn: async ({ signal }) =>
      (
        await api.read('/projects/{project_id}/baselines', readBaselines, {
          pathParams: { project_id: projectId },
          signal,
        })
      ).data,
  })
}
