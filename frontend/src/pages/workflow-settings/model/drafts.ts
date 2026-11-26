import { providerLabel } from '@/shared/config'
import type { ChangePreview } from '@/shared/ui'
import type {
  CalendarDraft,
  Catalog,
  CatalogInstance,
  MappingDraft,
  MappingRule,
  MappingVersion,
} from '../api/workflow-responses'

export const statusMeanings = {
  intake: 'Входящие (intake)',
  active: 'В работе (active)',
  wait: 'Ожидание (wait)',
  blocked: 'Блокировка (blocked)',
  done: 'Завершено (done)',
  cancelled: 'Отменено (cancelled)',
  duplicate: 'Дубликат (duplicate)',
  ignored: 'Игнорируется (ignored)',
}

export const resolutionMeanings = {
  accepted: 'Принято (accepted)',
  cancelled: statusMeanings.cancelled,
  duplicate: statusMeanings.duplicate,
  ignored: statusMeanings.ignored,
}

const ruleLabels: Record<MappingRule['rule_kind'], string> = {
  status: 'Статус',
  resolution: 'Исход',
  issue_type: 'Тип задачи',
  code_required_type: 'Требуется связь с MR',
  size_field: 'Поле размера',
  size_value: 'Размер',
  team_field: 'Поле команды',
  team_value: 'Команда',
  blocked_flag: 'Флаг блокировки',
  acceptance_gate: 'Этап приёмки',
  epic_field: 'Поле эпика',
  business_epic_value: 'Бизнес-эпик',
}

export function sourceLabel(instance: CatalogInstance) {
  return `${providerLabel(instance.provider)} · ${instance.instance_id}`
}

export function ruleLabel(rule: MappingRule, catalog: Catalog) {
  const instance = catalog.instances.find(
    (source) => source.instance_id === rule.instance_id,
  )
  const entry =
    instance &&
    (rule.rule_kind === 'status' || rule.rule_kind === 'acceptance_gate'
      ? instance.statuses
      : rule.rule_kind === 'resolution'
        ? instance.resolutions
        : rule.rule_kind === 'issue_type' ||
            rule.rule_kind === 'code_required_type'
          ? instance.issue_types
          : instance.fields
    ).find((value) => value.native_id === rule.native_ref)
  const name = entry ? `${entry.name} (${entry.native_id})` : rule.native_ref
  return `${ruleLabels[rule.rule_kind]} · ${name}${rule.native_value ? ` = ${rule.native_value}` : ''} · ${instance ? sourceLabel(instance) : rule.instance_id}`
}

export function newRule(
  rule_kind: MappingRule['rule_kind'],
  instance_id: string,
  native_ref: string,
  meaning = '',
): MappingRule {
  return {
    rule_kind,
    instance_id,
    native_ref,
    native_value: '',
    meaning,
    rank: null,
  }
}

export function suggestedMeaning(category: string | null | undefined) {
  switch (category?.toLowerCase() ?? '') {
    case 'new':
    case 'todo':
    case 'intake':
      return 'intake'
    case 'active':
    case 'indeterminate':
    case 'in_progress':
      return 'active'
    case 'done':
    case 'completed':
      return 'done'
    default:
      return undefined
  }
}

export function suggestedRules(catalog: Catalog) {
  return catalog.instances.flatMap((instance) =>
    instance.statuses.flatMap((status) => {
      const meaning = suggestedMeaning(status.source_category)
      return meaning
        ? [newRule('status', instance.instance_id, status.native_id, meaning)]
        : []
    }),
  )
}

export function mappingDraft(version: MappingVersion): MappingDraft {
  return {
    timezone: version.timezone,
    calendar_version_no: version.calendar_version_no,
    rules: version.rules,
  }
}

export function calendarDraft(version?: CalendarDraft): CalendarDraft {
  return {
    timezone: version?.timezone ?? 'UTC',
    workdays: version?.workdays ?? [],
    work_start: version?.work_start ?? null,
    work_end: version?.work_end ?? null,
    holidays: version?.holidays ?? [],
  }
}

function ruleKey(rule: MappingRule) {
  return JSON.stringify([
    rule.rule_kind,
    rule.instance_id,
    rule.native_ref,
    rule.native_value,
  ])
}

function ruleSummary(rule: MappingRule) {
  const meaning =
    rule.rule_kind === 'status'
      ? Object.entries(statusMeanings).find(
          ([code]) => code === rule.meaning,
        )?.[1]
      : rule.rule_kind === 'resolution'
        ? Object.entries(resolutionMeanings).find(
            ([code]) => code === rule.meaning,
          )?.[1]
        : rule.meaning
  return `${meaning === '' ? 'Выбрано' : (meaning ?? rule.meaning)}${rule.rank === null ? '' : ` · порядок ${String(rule.rank)}`}`
}

function addChange(
  changes: ChangePreview[],
  label: string,
  before: string,
  after: string,
) {
  if (before !== after) changes.push({ label, before, after })
}

export function mappingChanges(
  current: MappingDraft | undefined,
  draft: MappingDraft,
  catalog: Catalog,
) {
  const result: ChangePreview[] = []
  addChange(
    result,
    'Часовой пояс отчёта',
    current?.timezone ?? '—',
    draft.timezone,
  )
  addChange(
    result,
    'Рабочий календарь',
    current ? String(current.calendar_version_no ?? 'Без версии') : '—',
    String(draft.calendar_version_no ?? 'Без версии'),
  )
  const previous = new Map(current?.rules.map((rule) => [ruleKey(rule), rule]))
  for (const rule of draft.rules) {
    const key = ruleKey(rule)
    const old = previous.get(key)
    if (old?.meaning !== rule.meaning || old.rank !== rule.rank) {
      result.push({
        label: ruleLabel(rule, catalog),
        before: old ? ruleSummary(old) : 'Без правила',
        after: ruleSummary(rule),
      })
    }
    previous.delete(key)
  }
  for (const rule of previous.values()) {
    result.push({
      label: ruleLabel(rule, catalog),
      before: ruleSummary(rule),
      after: 'Без правила',
    })
  }
  return result
}

export const weekdayNames: Record<CalendarDraft['workdays'][number], string> = {
  mon: 'Пн',
  tue: 'Вт',
  wed: 'Ср',
  thu: 'Чт',
  fri: 'Пт',
  sat: 'Сб',
  sun: 'Вс',
}

function calendarFields(draft: CalendarDraft) {
  return [
    ['Часовой пояс', draft.timezone],
    [
      'Рабочие дни',
      Object.entries(weekdayNames)
        .filter(([day]) => draft.workdays.some((value) => value === day))
        .map(([, name]) => name)
        .join(', ') || '—',
    ],
    ['Начало дня', draft.work_start ?? '—'],
    ['Конец дня', draft.work_end ?? '—'],
    [
      'Праздники',
      draft.holidays
        .map((date) => date || 'Без даты')
        .toSorted()
        .join(', ') || '—',
    ],
  ] as const
}

export function calendarChanges(
  current: CalendarDraft | undefined,
  draft: CalendarDraft,
) {
  const previous = new Map(calendarFields(calendarDraft(current)))
  const result: ChangePreview[] = []
  for (const [label, after] of calendarFields(draft)) {
    addChange(
      result,
      label,
      current ? (previous.get(label) ?? '—') : '—',
      after,
    )
  }
  return result
}
