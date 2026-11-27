import type { RefObject } from 'react'
import { useQuery } from '@tanstack/react-query'
import type { ApiClient } from '@/shared/api'
import { ErrorState } from '@/shared/ui'
import { roleAreas } from '../api/users-api'
import type { AreaField, RoleInput } from '../api/users-api'
import {
  areaKinds,
  areaLabels,
  isRole,
  roleLabels,
  roles,
} from '../model/roles'
import type { AreaKind } from '../model/roles'

export interface NewRole {
  role: keyof typeof roleLabels | ''
  area: AreaKind
  id: string
}
export const emptyRole: NewRole = { role: '', area: 'organization', id: '' }

export function newAssignment(value: NewRole): RoleInput | undefined {
  if (
    !value.role ||
    !areaKinds(value.role).includes(value.area) ||
    (value.area !== 'organization' && !value.id)
  )
    return undefined
  return {
    role: value.role,
    team_id: value.area === 'team_id' ? value.id : null,
    project_id: value.area === 'project_id' ? value.id : null,
    service_id: value.area === 'service_id' ? value.id : null,
  }
}

export function NewRoleFields({
  api,
  value,
  roleRef,
  onChange,
}: {
  api: ApiClient
  value: NewRole
  roleRef: RefObject<HTMLSelectElement | null>
  onChange: (value: NewRole) => void
}) {
  const allowed = value.role ? areaKinds(value.role) : []
  return (
    <div className="cp-users__controls">
      <label>
        Роль
        <select
          ref={roleRef}
          value={value.role}
          onChange={({ target }) => {
            if (!target.value) onChange(emptyRole)
            else if (isRole(target.value)) {
              onChange({
                role: target.value,
                area: areaKinds(target.value)[0] ?? 'organization',
                id: '',
              })
            }
          }}
        >
          <option value="">Выберите роль</option>
          {roles.map((role) => (
            <option key={role} value={role}>
              {roleLabels[role]}
            </option>
          ))}
        </select>
      </label>
      <label>
        Область роли
        <select
          value={value.area}
          disabled={allowed.length < 2}
          onChange={({ target }) => {
            const area = allowed.find((kind) => kind === target.value)
            if (area) onChange({ ...value, area, id: '' })
          }}
        >
          {(allowed.length ? allowed : (['organization'] as const)).map(
            (kind) => (
              <option key={kind} value={kind}>
                {areaLabels[kind]}
              </option>
            ),
          )}
        </select>
      </label>
      {value.area !== 'organization' && (
        <AreaSelect
          api={api}
          field={value.area}
          id={value.id}
          onChange={(id) => {
            onChange({ ...value, id })
          }}
        />
      )}
    </div>
  )
}

function AreaSelect({
  api,
  field,
  id,
  onChange,
}: {
  api: ApiClient
  field: AreaField
  id: string
  onChange: (id: string) => void
}) {
  const query = useQuery(roleAreas(api, field))
  const options = Array.from(query.data?.values() ?? [])
  return (
    <>
      <label>
        {areaLabels[field]}
        <select
          value={id}
          disabled={!query.isSuccess || !options.length}
          onChange={({ target }) => {
            if (!target.value || query.data?.has(target.value))
              onChange(target.value)
          }}
        >
          <option value="">
            {query.isPending
              ? 'Загрузка областей'
              : options.length
                ? 'Выберите область роли'
                : 'Нет доступных областей'}
          </option>
          {options.map(({ id, name }) => (
            <option key={id} value={id}>
              {name}
            </option>
          ))}
        </select>
      </label>
      {query.isError && (
        <ErrorState
          reason="Не удалось загрузить области"
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      )}
    </>
  )
}
