import type { ChangePreview } from '@/shared/ui'
import type { Structure, StructureInput } from '../api/structure'

export const sections = {
  teams: 'Команды',
  projects: 'Проекты',
  services: 'Сервисы',
  components: 'Компоненты',
  team_projects: 'Команды и проекты',
  team_members: 'Участники команд',
  project_areas: 'Области источников проектов',
}

export type Section = keyof typeof sections
export type NamedSection = 'teams' | 'projects' | 'services'
export type LinkSection = 'team_projects' | 'team_members' | 'project_areas'
type Row<K extends Section> = Draft[K][number]
export interface Draft extends Omit<StructureInput, 'components'> {
  components: (Omit<Structure['components'][number], 'id'> & {
    id: string | null
  })[]
}

export const noInterval = { valid_from: null, valid_to: null }

export function fromStructure({
  teams,
  projects,
  services,
  components,
  team_projects,
  team_members,
  project_areas,
}: Structure): Draft {
  return {
    teams,
    projects,
    services,
    components,
    team_projects: team_projects.map(({ team_id, project_id }) => ({
      team_id,
      project_id,
      ...noInterval,
    })),
    team_members: team_members.map(({ team_id, person_id }) => ({
      team_id,
      person_id,
      ...noInterval,
    })),
    project_areas: project_areas.map(({ project_id, area_id }) => ({
      project_id,
      area_id,
      ...noInterval,
    })),
  }
}

function trimNames(rows: Draft['teams']) {
  return rows.map((row) => ({ ...row, name: row.name.trim() }))
}

export function toInput(draft: Draft): StructureInput {
  return {
    ...draft,
    teams: trimNames(draft.teams),
    projects: trimNames(draft.projects),
    services: trimNames(draft.services),
    components: draft.components.flatMap(
      ({ id, service_id, repository_area_id, path_prefix, status }) =>
        status === 'proposed'
          ? []
          : [{ id, service_id, repository_area_id, path_prefix, status }],
    ),
  }
}

export function editRow<K extends Section>(
  draft: Draft,
  section: K,
  index: number,
  row?: Row<K>,
): Draft {
  const rows: Row<K>[] = draft[section].slice()
  if (row) rows[index] = row
  else rows.splice(index, 1)
  return { ...draft, [section]: rows }
}

export const fieldLabels: Record<string, string> = {
  id: 'Запись',
  name: 'Имя',
  status: 'Статус',
  team_id: 'Команда',
  project_id: 'Проект',
  service_id: 'Сервис',
  person_id: 'Участник',
  repository_area_id: 'Репозиторий',
  area_id: 'Область источника',
  path_prefix: 'Префикс пути',
  valid_from: 'Интервал связи',
  valid_to: 'Интервал связи',
}

export const statuses = {
  active: 'Активна',
  archived: 'В архиве',
  confirmed: 'Подтверждён',
  proposed: 'Предложен',
  rejected: 'Отклонён',
}

type DraftRow = Row<Section>
type NameLookup = (id: string) => string

export function describeRow(row: DraftRow, nameOf: NameLookup): string {
  if ('name' in row) return row.name || 'Без имени'
  if ('path_prefix' in row) {
    return [
      nameOf(row.service_id),
      nameOf(row.repository_area_id),
      row.path_prefix || 'Весь репозиторий',
      statuses[row.status],
    ].join(' · ')
  }
  if ('person_id' in row)
    return [nameOf(row.team_id), nameOf(row.person_id)].join(' · ')
  if ('area_id' in row)
    return [nameOf(row.project_id), nameOf(row.area_id)].join(' · ')
  return [nameOf(row.team_id), nameOf(row.project_id)].join(' · ')
}

function rowKey(row: DraftRow) {
  if ('id' in row) return row.id
  if ('person_id' in row) return JSON.stringify([row.team_id, row.person_id])
  if ('area_id' in row) return JSON.stringify([row.project_id, row.area_id])
  return JSON.stringify([row.team_id, row.project_id])
}

function keyedRows<T extends DraftRow>(rows: T[]) {
  const occurrences = new Map<string, number>()
  const result = new Map<string, T>()
  for (const row of rows) {
    const key = rowKey(row)
    if (key === null) continue
    const occurrence = occurrences.get(key) ?? 0
    occurrences.set(key, occurrence + 1)
    result.set(JSON.stringify([key, occurrence]), row)
  }
  return result
}

function fields(row: DraftRow): Record<string, string | null> {
  return { ...row }
}

function equal(a: DraftRow, b: DraftRow) {
  const other = fields(b)
  return Object.entries(fields(a)).every(([key, value]) => other[key] === value)
}

function compareRows(
  before: DraftRow[],
  after: DraftRow[],
  section: Section,
  nameOf: NameLookup,
): ChangePreview[] {
  const old = keyedRows(before)
  const next = keyedRows(after)
  const changes: ChangePreview[] = []
  for (const [key, row] of old) {
    const updated = next.get(key)
    if (!updated)
      changes.push({
        label: sections[section],
        before: describeRow(row, nameOf),
        after: 'Удалено',
      })
    else if (!equal(row, updated)) {
      const current = fields(updated)
      for (const [field, value] of Object.entries(fields(row))) {
        if (current[field] === value) continue
        const format = (text: string | null | undefined) => {
          if (field === 'status' && text && Object.hasOwn(statuses, text)) {
            return (
              Object.entries(statuses).find(
                ([status]) => status === text,
              )?.[1] ?? text
            )
          }
          if (field === 'path_prefix')
            return text === '' ? 'Весь репозиторий' : (text ?? 'Не указано')
          return text
            ? field.endsWith('_id')
              ? nameOf(text)
              : text
            : 'Не указано'
        }
        changes.push({
          label: [
            sections[section],
            describeRow(row, nameOf),
            fieldLabels[field] ?? field,
          ].join(' · '),
          before: format(value),
          after: format(current[field]),
        })
      }
    }
  }
  const newRows = [
    ...after.filter((row) => rowKey(row) === null),
    ...Array.from(next, ([key, row]) => (old.has(key) ? [] : [row])).flat(),
  ]
  for (const row of newRows) {
    changes.push({
      label: sections[section],
      before: 'Нет',
      after: describeRow(row, nameOf),
    })
  }
  return changes
}

export function compare(
  before: Draft,
  after: Draft,
  nameOf: NameLookup,
): ChangePreview[] {
  return (Object.keys(sections) as Section[]).flatMap((section) =>
    compareRows(before[section], after[section], section, nameOf),
  )
}

function mergeRows<T extends DraftRow>(
  base: T[],
  local: T[],
  latest: T[],
): T[] {
  const original = keyedRows(base)
  const edited = keyedRows(local)
  const remote = keyedRows(latest)
  const merged: T[] = []
  for (const [key, current] of remote) {
    const before = original.get(key)
    const change = edited.get(key)
    if (!before) {
      merged.push(current)
    } else if (change) {
      // Only replay fields that changed locally. The new preview shows any
      // overlapping edit against the server value before it can be replaced.
      const row = { ...current }
      for (const field of Object.keys(change) as (keyof T)[]) {
        if (before[field] !== change[field]) row[field] = change[field]
      }
      merged.push(row)
    }
  }
  for (const [key, row] of edited) {
    if (remote.has(key)) continue
    const before = original.get(key)
    if (!before) merged.push(row)
    // An edited record removed by the server cannot be written with its old ID.
    else if (!equal(before, row)) throw new Error('Edited record was removed')
  }
  merged.push(...local.filter((row) => 'id' in row && row.id === null))
  return merged
}

// A response can be lost after an insert. Consume observed new rows once so
// resuming the draft cannot insert another copy of the same record.
function pendingCreations<T extends DraftRow & { id: string | null }>(
  base: T[],
  local: T[],
  latest: T[],
) {
  const previousIds = new Set(base.map(({ id }) => id))
  const available = new Map<string, number>()
  function signature(row: T) {
    const values = fields(row)
    delete values.id
    if (typeof values.name === 'string') values.name = values.name.trim()
    return JSON.stringify(
      Object.entries(values).sort(([a], [b]) => a.localeCompare(b)),
    )
  }
  for (const row of latest) {
    if (previousIds.has(row.id)) continue
    const key = signature(row)
    available.set(key, (available.get(key) ?? 0) + 1)
  }
  return local.filter((row) => {
    if (row.id !== null) return true
    const key = signature(row)
    const count = available.get(key) ?? 0
    if (!count) return true
    available.set(key, count - 1)
    return false
  })
}

export function reconcileCreations(
  base: Draft,
  local: Draft,
  latest: Draft,
): Draft {
  return {
    ...local,
    teams: pendingCreations(base.teams, local.teams, latest.teams),
    projects: pendingCreations(base.projects, local.projects, latest.projects),
    services: pendingCreations(base.services, local.services, latest.services),
    components: pendingCreations(
      base.components,
      local.components,
      latest.components,
    ),
  }
}

export function rebase(base: Draft, local: Draft, latest: Draft): Draft {
  return {
    teams: mergeRows(base.teams, local.teams, latest.teams),
    projects: mergeRows(base.projects, local.projects, latest.projects),
    services: mergeRows(base.services, local.services, latest.services),
    components: mergeRows(base.components, local.components, latest.components),
    team_projects: mergeRows(
      base.team_projects,
      local.team_projects,
      latest.team_projects,
    ),
    team_members: mergeRows(
      base.team_members,
      local.team_members,
      latest.team_members,
    ),
    project_areas: mergeRows(
      base.project_areas,
      local.project_areas,
      latest.project_areas,
    ),
  }
}
