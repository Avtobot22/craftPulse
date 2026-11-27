import type { Choice } from '../api/structure'
import { sections } from '../model/structure-draft'
import type { Draft, LinkSection } from '../model/structure-draft'
import { StructureTable } from './structure-table'

export interface Catalog {
  areas: (Choice & { kind: string })[]
  people: Choice[]
  areaProblem?: string | undefined
  peopleProblem?: string | undefined
}

export const linkSections = [
  'team_projects',
  'team_members',
  'project_areas',
] as const

export const linkColumns = {
  team_projects: ['Команда', 'Проект'],
  team_members: ['Команда', 'Участник'],
  project_areas: ['Проект', 'Область источника'],
}

export function LinkTables({
  draft,
  selected,
  catalog,
  nameOf,
  onAdd,
  onSelect,
  owner,
}: {
  draft: Draft
  selected: { section: string; index: number }
  catalog: Catalog
  nameOf: (id: string) => string
  onAdd: (section: LinkSection) => void
  onSelect: (section: LinkSection, index: number) => void
  owner?: { kind: 'teams' | 'projects'; id: string } | undefined
}) {
  const hasTeam = draft.teams.some(({ id }) => id !== null)
  const hasProject = draft.projects.some(({ id }) => id !== null)
  const saveFirst = 'Сначала сохраните запись'
  const unavailable = {
    team_projects: !hasTeam || !hasProject ? saveFirst : undefined,
    team_members: !hasTeam
      ? saveFirst
      : (catalog.peopleProblem ??
        (!catalog.people.length
          ? 'В организации пока нет участников'
          : undefined)),
    project_areas:
      catalog.areaProblem ??
      (!catalog.areas.length
        ? 'Источник не настроен'
        : !hasProject
          ? saveFirst
          : undefined),
  }
  const visibleSections =
    owner?.kind === 'teams'
      ? linkSections.filter((section) => section !== 'project_areas')
      : owner?.kind === 'projects'
        ? linkSections.filter((section) => section !== 'team_members')
        : linkSections
  return visibleSections.map((section) => (
    <StructureTable
      key={section}
      title={sections[section]}
      columns={linkColumns[section]}
      activeIndex={selected.section === section ? selected.index : undefined}
      addLabel="Добавить связь"
      unavailable={unavailable[section]}
      onAdd={() => {
        onAdd(section)
      }}
      rows={draft[section].flatMap((row, index) => {
        if (
          owner?.kind === 'teams' &&
          (!('team_id' in row) || row.team_id !== owner.id)
        )
          return []
        if (
          owner?.kind === 'projects' &&
          (!('project_id' in row) || row.project_id !== owner.id)
        )
          return []
        const fields =
          'person_id' in row
            ? [row.team_id, row.person_id]
            : 'area_id' in row
              ? [row.project_id, row.area_id]
              : [row.team_id, row.project_id]
        const cells = fields.map(nameOf)
        return [
          {
            index,
            cells,
            actionLabel: 'Изменить связь ' + cells.join(' · '),
            onEdit: () => {
              onSelect(section, index)
            },
          },
        ]
      })}
    />
  ))
}
