import type { components } from './generated/schema'
import {
  InvalidResponse,
  readArray,
  readBoolean,
  readNullableString,
  readRecord,
  readString,
} from './response'

type Schema = components['schemas']

// Readers accept new string codes without losing the wire value.
export type OpenCodes<T> = T extends string
  ? string
  : T extends (infer Item)[]
    ? OpenCodes<Item>[]
    : T extends object
      ? { [K in keyof T]: OpenCodes<T[K]> }
      : T

export function readNumber(value: unknown): number {
  if (typeof value !== 'number' || !Number.isFinite(value))
    throw new InvalidResponse()
  return value
}

export function readNullableNumber(value: unknown): number | null {
  return value === null ? null : readNumber(value)
}

export function readObjectRef(value: unknown): OpenCodes<Schema['ObjectRef']> {
  const raw = readRecord(value)
  return {
    kind: readString(raw.kind),
    id: readString(raw.id),
    display_key: readNullableString(raw.display_key),
    title: readNullableString(raw.title),
    source_url: readNullableString(raw.source_url),
  }
}

export function readActor(value: unknown): OpenCodes<Schema['ActorRef']> {
  const raw = readRecord(value)
  return {
    account_id: readNullableString(raw.account_id),
    kind: readString(raw.kind),
    display_name: readNullableString(raw.display_name),
  }
}

export function readLink(value: unknown): OpenCodes<Schema['Link']> {
  const raw = readRecord(value)
  return {
    id: readString(raw.id),
    link_type: readString(raw.link_type),
    method: readString(raw.method),
    status: readString(raw.status),
    from_object_id: readString(raw.from_object_id),
    to: readObjectRef(raw.to),
    decided_at: readNullableString(raw.decided_at),
  }
}

export function readAccessNotes(
  value: unknown,
): OpenCodes<Schema['AccessNote']>[] {
  return readArray(value, (item) => {
    const raw = readRecord(item)
    return {
      code: readString(raw.code),
      ...(raw.connection_id === undefined
        ? {}
        : {
            connection_id: readNullableString(raw.connection_id),
          }),
    }
  })
}

export function readScopedPage(raw: Record<string, unknown>) {
  return {
    next_cursor: readNullableString(raw.next_cursor),
    access_limited: readBoolean(raw.access_limited),
    access_notes: readAccessNotes(raw.access_notes),
  }
}

export function readMetricValue(
  value: unknown,
): OpenCodes<Schema['MetricValue']> {
  const raw = readRecord(value)
  const bound = raw.bound === null ? null : readRecord(raw.bound)
  return {
    state: readString(raw.state),
    value: readNullableNumber(raw.value),
    unit: readString(raw.unit),
    numerator: readNullableNumber(raw.numerator),
    denominator: readNullableNumber(raw.denominator),
    n: readNullableNumber(raw.n),
    bound: bound && {
      kind: readString(bound.kind),
      value: readNumber(bound.value),
    },
    low_sample: readBoolean(raw.low_sample),
    excluded_count: readNumber(raw.excluded_count),
    missing_count: readNumber(raw.missing_count),
    reason_code: readNullableString(raw.reason_code),
  }
}

export function readLocation(value: unknown): Schema['CodeLocation'] {
  const raw = value === null ? null : readRecord(value)
  return (
    raw && {
      path: readString(raw.path),
      line_start: readNullableNumber(raw.line_start),
      line_end: readNullableNumber(raw.line_end),
    }
  )
}

export function readStringMap(value: unknown): Record<string, string> {
  return Object.fromEntries(
    Object.entries(readRecord(value)).map(([key, item]) => [
      key,
      readString(item),
    ]),
  )
}

export function readCursorPage<T>(
  value: unknown,
  readItem: (item: unknown) => T,
) {
  const raw = readRecord(value)
  return {
    data: readArray(raw.data, readItem),
    next_cursor: readNullableString(raw.next_cursor),
  }
}
