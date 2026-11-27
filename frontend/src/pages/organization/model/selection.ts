import { sections } from './structure-draft'
import type { Draft, Section } from './structure-draft'

export const tabs = [
  { id: 'teams', label: 'Команды', section: 'teams' },
  { id: 'projects', label: 'Проекты', section: 'projects' },
  { id: 'services', label: 'Сервисы и компоненты', section: 'services' },
  { id: 'links', label: 'Связи', section: 'team_projects' },
] as const

export function tabFor(section: Section) {
  if (section === 'components') return 'services'
  if (section.includes('_')) return 'links'
  return section
}

export function recordKey(row: Draft[Section][number], index: number) {
  return 'id' in row && row.id ? row.id : String(index)
}

export function readSelection(params: URLSearchParams, draft: Draft) {
  const tab = tabs.find(({ id }) => id === params.get('tab')) ?? tabs[0]
  const requested = params.get('section') ?? tab.section
  const section =
    isSection(requested) && tabFor(requested) === tab.id
      ? requested
      : tab.section
  const entry = params.get('record')
  const index = draft[section].findIndex(
    (row, position) => recordKey(row, position) === entry,
  )
  return { tab, section, index }
}

function isSection(value: string): value is Section {
  return Object.hasOwn(sections, value)
}
