import type {
  ApiResponse,
  DocumentationRoot,
  DocumentationRoots,
} from '@/shared/api'
import type { ChangeProposal, ChangeValidation } from '@/shared/ui'

export type RootFields = Omit<DocumentationRoot, 'id'>

export const gitProfile = 'git-markdown-openapi'
export const noOwner = 'Владелец не назначен'
export const audiences = {
  team: 'Команда',
  organization: 'Организация',
  external: 'Внешняя аудитория',
} as const

export function audienceLabel(code: string) {
  switch (code) {
    case 'team':
    case 'organization':
    case 'external':
      return audiences[code]
    default:
      return code
  }
}

export function rootFields({
  area_id,
  path_prefix,
  profile_id,
  owner_person_id,
  audience,
}: RootFields): RootFields {
  return { area_id, path_prefix, profile_id, owner_person_id, audience }
}

function rootKey(root: RootFields) {
  return JSON.stringify([root.area_id, root.path_prefix])
}

export function rootProposal<Draft extends RootFields[]>(
  draft: Draft,
  baseline: ApiResponse<DocumentationRoots>,
  repositories: ReadonlyMap<string, string> | undefined,
): ChangeProposal<Draft> {
  const before = new Map(
    baseline.data.data.map((root) => [rootKey(root), rootFields(root)]),
  )
  const after = new Map(draft.map((root) => [rootKey(root), root]))
  const describe = (root: RootFields | undefined) =>
    root
      ? [
          root.profile_id,
          root.owner_person_id ?? noOwner,
          audienceLabel(root.audience),
        ].join(' · ')
      : 'Корня нет'
  return {
    etag: baseline.etag ?? '',
    draft,
    changes: [...new Set([...before.keys(), ...after.keys()])].flatMap(
      (key) => {
        const oldRoot = before.get(key)
        const newRoot = after.get(key)
        const root = oldRoot ?? newRoot
        if (!root || JSON.stringify(oldRoot) === JSON.stringify(newRoot))
          return []
        const repository = repositories?.get(root.area_id) ?? root.area_id
        const action = oldRoot
          ? newRoot
            ? 'Изменение'
            : 'Удаление'
          : 'Добавление'
        return [
          {
            label: `${action} · ${repository} / ${root.path_prefix}`,
            before: describe(oldRoot),
            after: describe(newRoot),
          },
        ]
      },
    ),
  }
}

export function validationIssues(validation: ChangeValidation) {
  const issues = [...validation.fields].flatMap(([field, errors]) =>
    errors.map(({ code }) => {
      switch (code) {
        case 'outside_scope':
          return { field, message: 'Корень вне вашей области.' }
        case 'unknown_reference':
          return { field, message: 'Объект с таким ID недоступен.' }
        case 'duplicate':
          return { field, message: 'Этот путь в репозитории уже добавлен.' }
        case 'invalid_format':
          return { field, message: 'Проверьте формат значения.' }
        case 'not_allowed_value':
          return { field, message: 'Выберите поддерживаемое значение.' }
        case 'too_long':
          return { field, message: 'Значение слишком длинное.' }
        case 'required':
          return { field, message: 'Заполните поле.' }
        case 'out_of_range':
          return { field, message: 'Значение вне допустимого диапазона.' }
        case 'unknown_field':
        case 'unknown':
          return { field, message: 'Проверьте значение.' }
      }
    }),
  )
  return issues.length
    ? issues
    : [
        {
          field: '/data',
          message: 'Сервис отклонил набор корней. Проверьте значения.',
        },
      ]
}

export function rootFieldId(index: number, field: keyof RootFields) {
  return `root-${String(index)}-${field}`
}

export function validationTarget(pointer: string) {
  const match =
    /^\/data\/(\d+)\/(area_id|path_prefix|profile_id|owner_person_id|audience)$/u.exec(
      pointer,
    )
  return match?.[1] && match[2] ? `root-${match[1]}-${match[2]}` : undefined
}
