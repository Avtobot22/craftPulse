import type { ChangeValidation } from '@/shared/ui'
import { sections } from './structure-draft'
import type { Draft, Section } from './structure-draft'

export interface Issue {
  section?: Section | undefined
  index?: number | undefined
  field?: string | undefined
  message: string
}

const limits = {
  teams: 500,
  projects: 2000,
  services: 2000,
  components: 5000,
  team_projects: 5000,
  team_members: 5000,
  project_areas: 5000,
}

export function validate(draft: Draft, base: Draft): Issue[] {
  const issues: Issue[] = []
  for (const section of ['teams', 'projects', 'services'] as const) {
    draft[section].forEach(({ name }, index) => {
      if (!name.trim())
        issues.push({ section, index, field: 'name', message: 'Укажите имя' })
      else if (Array.from(name.trim()).length > 200)
        issues.push({
          section,
          index,
          field: 'name',
          message: 'Имя должно содержать не более 200 символов',
        })
    })
  }
  const proposals = new Map(base.components.map((row) => [row.id, row]))
  draft.components.forEach((row, index) => {
    if (row.status === 'proposed') {
      const original = proposals.get(row.id)
      if (
        original &&
        (original.path_prefix !== row.path_prefix ||
          original.service_id !== row.service_id ||
          original.repository_area_id !== row.repository_area_id)
      ) {
        issues.push({
          section: 'components',
          index,
          field: 'status',
          message: 'Подтвердите или отклоните изменённое предложение',
        })
      }
      return
    }
    if (!row.service_id)
      issues.push({
        section: 'components',
        index,
        field: 'service_id',
        message: 'Выберите сервис',
      })
    if (!row.repository_area_id)
      issues.push({
        section: 'components',
        index,
        field: 'repository_area_id',
        message: 'Выберите репозиторий',
      })
    if (Array.from(row.path_prefix).length > 1024)
      issues.push({
        section: 'components',
        index,
        field: 'path_prefix',
        message: 'Префикс пути должен содержать не более 1024 символов',
      })
  })
  for (const section of [
    'team_projects',
    'team_members',
    'project_areas',
  ] as const) {
    draft[section].forEach((row, index) => {
      for (const [field, value] of Object.entries(row)) {
        if (value === '')
          issues.push({ section, index, field, message: 'Выберите значение' })
      }
    })
  }
  for (const [key, limit] of Object.entries(limits)) {
    if (!isSection(key)) continue
    const count =
      key === 'components'
        ? draft.components.filter(({ status }) => status !== 'proposed').length
        : draft[key].length
    if (count > limit)
      issues.push({
        section: key,
        message: 'Допустимо записей: ' + String(limit),
      })
  }
  return issues
}

function isSection(value: string): value is Section {
  return Object.hasOwn(sections, value)
}

const intervalFields = {
  team_projects: 'project_id',
  team_members: 'person_id',
  project_areas: 'area_id',
} as const

export function serverIssues(
  { fields }: ChangeValidation,
  draft: Draft,
): Issue[] {
  const componentIndices = draft.components.flatMap(({ status }, index) =>
    status === 'proposed' ? [] : [index],
  )
  const issues: Issue[] = []
  for (const [pointer, errors] of fields) {
    const [, collection = '', row, field] = pointer.split('/')
    const section = isSection(collection) ? collection : undefined
    const position =
      row !== undefined && /^\d+$/u.test(row) ? Number(row) : undefined
    const index =
      section === 'components' && position !== undefined
        ? componentIndices[position]
        : position
    const intervalField =
      section === 'team_projects' ||
      section === 'team_members' ||
      section === 'project_areas'
        ? intervalFields[section]
        : undefined
    for (const { code } of errors) {
      let message = 'Проверьте значение'
      if (code === 'required') message = 'Укажите значение'
      if (code === 'invalid_format' && field === 'path_prefix')
        message = 'Проверьте путь внутри репозитория'
      if (code === 'unknown_reference')
        message = 'Выбранная запись больше недоступна'
      if (code === 'duplicate') {
        message = intervalField
          ? 'Интервалы связи пересекаются'
          : 'Такая запись уже есть'
      }
      const target =
        index !== undefined &&
        intervalField &&
        (!field || field === 'valid_from' || field === 'valid_to')
          ? intervalField
          : field
      issues.push({ section, index, field: target, message })
    }
  }
  return issues.length
    ? issues
    : [{ message: 'Сервер отклонил структуру. Проверьте введённые значения.' }]
}
