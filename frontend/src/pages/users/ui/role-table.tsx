import { Button } from '@gravity-ui/uikit'
import { StatusTime } from '@/shared/ui'
import type { RoleInput } from '../api/users-api'
import {
  areaFields,
  areaLabel,
  isRole,
  roleKey,
  roleLabel,
} from '../model/roles'
import type { AreaCatalog } from '../model/roles'

export function RoleTable({
  draft,
  assigned,
  catalog,
  issues,
  onRemove,
}: {
  draft: RoleInput[]
  assigned: ReadonlyMap<string, string>
  catalog: AreaCatalog
  issues: ReadonlyMap<string, string>
  onRemove: (role: RoleInput) => void
}) {
  if (!draft.length) return <p>Нет назначений ролей</p>
  return (
    <div
      className="cp-users__scroll"
      role="region"
      aria-label="Назначения ролей"
      tabIndex={0}
    >
      <table className="cp-users__table" aria-label="Роли и области">
        <thead>
          <tr>
            <th scope="col">Роль</th>
            <th scope="col">Область</th>
            <th scope="col">Дата назначения</th>
            <th scope="col">Действия</th>
          </tr>
        </thead>
        <tbody>
          {draft.map((item, index) => {
            const key = roleKey(item)
            const path = '/data/' + String(index)
            const roleIssue = issues.get(path + '/role')
            const areaIssues = [
              issues.get(path),
              ...areaFields.map((field) => issues.get(path + '/' + field)),
            ].filter(Boolean)
            const date = assigned.get(key)
            const label = roleLabel(item.role)
            const area = areaLabel(item, catalog)
            return (
              <tr key={key}>
                <th scope="row">
                  {label}
                  {roleIssue && (
                    <p className="cp-users__error">Ошибка роли. {roleIssue}</p>
                  )}
                </th>
                <td>
                  {area}
                  {areaIssues.length > 0 && (
                    <p className="cp-users__error">
                      Ошибка назначения. {areaIssues.join(' ')}
                    </p>
                  )}
                </td>
                <td>
                  {date ? <StatusTime value={date} /> : 'После сохранения'}
                </td>
                <td>
                  {isRole(item.role) && (
                    <Button
                      size="xl"
                      aria-label={'Снять назначение: ' + label + ', ' + area}
                      onClick={() => {
                        onRemove(item)
                      }}
                    >
                      Снять
                    </Button>
                  )}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
