import {
  InvalidResponse,
  readArray,
  readMetricValue,
  readNullableNumber,
  readNullableString,
  readNumber,
  readObjectRef,
  readRecord,
  readString,
} from '@/shared/api'
import type { components, OpenCodes } from '@/shared/api'

type Schemas = components['schemas']
export type Catalog = OpenCodes<Schemas['WorkflowCatalog']>
export type CatalogInstance = Catalog['instances'][number]
export type MappingDraft = Schemas['MappingDraft']
export type MappingRule = Schemas['MappingRule']
export type MappingVersion = Schemas['MappingVersion']
export type MappingPreview = OpenCodes<Schemas['MappingPreview']>
export type CalendarDraft = Schemas['CalendarDraft']
export type CalendarVersion = Schemas['CalendarVersion']

function readCatalogEntry(value: unknown) {
  const raw = readRecord(value)
  return { native_id: readString(raw.native_id), name: readString(raw.name) }
}

function readStatusRef(value: unknown) {
  const raw = readRecord(value)
  return {
    instance_id: readString(raw.instance_id),
    native_id: readString(raw.native_id),
  }
}

export function readCatalog(value: unknown): Catalog {
  const raw = readRecord(value)
  return {
    instances: readArray(raw.instances, (item) => {
      const instance = readRecord(item)
      return {
        instance_id: readString(instance.instance_id),
        provider: readString(instance.provider),
        statuses: readArray(instance.statuses, (value) => ({
          ...readCatalogEntry(value),
          source_category: readNullableString(
            readRecord(value).source_category,
          ),
        })),
        resolutions: readArray(instance.resolutions, readCatalogEntry),
        issue_types: readArray(instance.issue_types, readCatalogEntry),
        fields: readArray(instance.fields, (value) => ({
          ...readCatalogEntry(value),
          value_type: readString(readRecord(value).value_type),
        })),
      }
    }),
    unmapped_statuses: readArray(raw.unmapped_statuses, readStatusRef),
  }
}

function readRuleKind(value: unknown): MappingRule['rule_kind'] {
  switch (value) {
    case 'status':
    case 'resolution':
    case 'issue_type':
    case 'code_required_type':
    case 'size_field':
    case 'size_value':
    case 'team_field':
    case 'team_value':
    case 'blocked_flag':
    case 'acceptance_gate':
    case 'epic_field':
    case 'business_epic_value':
      return value
    default:
      throw new InvalidResponse()
  }
}

function readMappingRule(value: unknown): MappingRule {
  const raw = readRecord(value)
  return {
    rule_kind: readRuleKind(raw.rule_kind),
    instance_id: readString(raw.instance_id),
    native_ref: readString(raw.native_ref),
    native_value: readString(raw.native_value),
    meaning: readString(raw.meaning),
    rank: readNullableNumber(raw.rank),
  }
}

function readConfirmation(raw: Record<string, unknown>) {
  return {
    version_no: readNumber(raw.version_no),
    base_version_no: readNullableNumber(raw.base_version_no),
    confirmed_by: readString(raw.confirmed_by),
    confirmed_at: readString(raw.confirmed_at),
  }
}

export function readMappingVersion(value: unknown): MappingVersion {
  const raw = readRecord(value)
  return {
    ...readConfirmation(raw),
    timezone: readString(raw.timezone),
    calendar_version_no: readNullableNumber(raw.calendar_version_no),
    rules: readArray(raw.rules, readMappingRule),
  }
}

export const weekdays = [
  'mon',
  'tue',
  'wed',
  'thu',
  'fri',
  'sat',
  'sun',
] as const

export function readCalendarVersion(value: unknown): CalendarVersion {
  const raw = readRecord(value)
  return {
    ...readConfirmation(raw),
    timezone: readString(raw.timezone),
    workdays: readArray(raw.workdays, (value) => {
      const weekday = weekdays.find((day) => day === value)
      if (!weekday) throw new InvalidResponse()
      return weekday
    }),
    work_start: readNullableString(raw.work_start),
    work_end: readNullableString(raw.work_end),
    holidays: readArray(raw.holidays, readString),
  }
}

export function readMappingPreview(value: unknown): MappingPreview {
  const raw = readRecord(value)
  const period = readRecord(raw.period)
  return {
    as_of: readString(raw.as_of),
    period: { from: readString(period.from), to: readString(period.to) },
    tasks: readArray(raw.tasks, (value) => {
      const task = readRecord(value)
      return {
        task: readObjectRef(task.task),
        started_at: readNullableString(task.started_at),
        completed_at: readNullableString(task.completed_at),
        outcome: readString(task.outcome),
        blocked_seconds: readNullableNumber(task.blocked_seconds),
        task_type: readNullableString(task.task_type),
        size_label: readNullableString(task.size_label),
        unmapped: readArray(task.unmapped, readString),
      }
    }),
    control_values: readArray(raw.control_values, (value) => {
      const control = readRecord(value)
      return {
        metric_id: readString(control.metric_id),
        value: readMetricValue(control.value),
      }
    }),
    unmapped_statuses: readArray(raw.unmapped_statuses, readStatusRef),
  }
}
