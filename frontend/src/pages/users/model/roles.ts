import type { components, Team } from '@/shared/api'
import type { ChangePreview } from '@/shared/ui'
import type { AreaField, RoleInput } from '../api/users-api'

type Role = components['schemas']['Role']
export const roleLabels: Record<Role, string> = {
  administrator: 'Администратор',
  dev_lead: 'Руководитель разработки',
  project_manager: 'Менеджер проекта',
  tech_lead: 'Технический лидер',
  developer: 'Разработчик',
  process_owner: 'Владелец процесса',
  service_owner: 'Владелец сервиса',
  security_access: 'Доступ к данным безопасности',
  wiki_access: 'Доступ к wiki',
}
export const roles = Object.keys(roleLabels).filter(isRole)

export function isRole(value: string): value is Role {
  return Object.hasOwn(roleLabels, value)
}

export function roleLabel(role: string) {
  return isRole(role) ? roleLabels[role] : 'Неизвестная роль (' + role + ')'
}

export const areaLabels = {
  organization: 'Вся организация',
  team_id: 'Команда',
  project_id: 'Проект',
  service_id: 'Сервис',
} as const

export type AreaKind = keyof typeof areaLabels
export const areaFields = ['team_id', 'project_id', 'service_id'] as const
export type AreaCatalog = Partial<Record<AreaField, ReadonlyMap<string, Team>>>

export function areaKinds(role: Role): readonly AreaKind[] {
  switch (role) {
    case 'administrator':
      return ['organization']
    case 'process_owner':
      return ['team_id']
    case 'service_owner':
      return ['service_id']
    case 'dev_lead':
    case 'project_manager':
    case 'tech_lead':
    case 'developer':
    case 'security_access':
    case 'wiki_access':
      return ['organization', ...areaFields]
  }
}

export function areaLabel(role: RoleInput, catalog: AreaCatalog) {
  const labels = areaFields.flatMap((field) => {
    const id = role[field]
    return id
      ? [areaLabels[field] + ' · ' + (catalog[field]?.get(id)?.name ?? id)]
      : []
  })
  return labels.join(', ') || areaLabels.organization
}

export function roleInput({
  role,
  team_id,
  project_id,
  service_id,
}: RoleInput): RoleInput {
  return { role, team_id, project_id, service_id }
}

export function roleKey({ role, team_id, project_id, service_id }: RoleInput) {
  return JSON.stringify([role, team_id, project_id, service_id])
}

function difference(first: readonly RoleInput[], second: readonly RoleInput[]) {
  const keys = new Set(second.map(roleKey))
  return first.filter((item) => !keys.has(roleKey(item)))
}

export function rebaseRoles(
  base: RoleInput[],
  draft: RoleInput[],
  current: RoleInput[],
) {
  const removed = new Set(difference(base, draft).map(roleKey))
  return [
    ...current.filter((item) => !removed.has(roleKey(item))),
    ...difference(difference(draft, base), current),
  ]
}

export function rolesChanged(base: RoleInput[], draft: RoleInput[]) {
  return (
    difference(base, draft).length > 0 || difference(draft, base).length > 0
  )
}

export function roleChanges(
  base: RoleInput[],
  draft: RoleInput[],
  catalog: AreaCatalog,
): ChangePreview[] {
  return [
    ...difference(base, draft).map((item) => ({
      label: 'Снять назначение',
      before: describeRole(item, catalog),
      after: 'Нет назначения',
    })),
    ...difference(draft, base).map((item) => ({
      label: 'Добавить назначение',
      before: 'Нет назначения',
      after: describeRole(item, catalog),
    })),
  ]
}

export function describeRole(item: RoleInput, catalog: AreaCatalog) {
  return roleLabel(item.role) + ' · ' + areaLabel(item, catalog)
}
