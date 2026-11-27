import { Button } from '@gravity-ui/uikit'
import { technicalIdPattern } from '@/shared/config'
import { audiences, gitProfile, noOwner, rootFieldId } from '../model/root-set'
import type { RootFields as Fields } from '../model/root-set'

export function RootFields({
  index,
  fields,
  issues,
  edit,
  remove,
}: {
  index: number
  fields: Fields
  issues: ReadonlySet<string>
  edit: (field: keyof Fields, value: string) => void
  remove: () => void
}) {
  const field = (name: keyof Fields) => ({
    id: rootFieldId(index, name),
    'aria-invalid': issues.has(rootFieldId(index, name)) || undefined,
    value: fields[name] ?? '',
    onChange: (
      event: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>,
    ) => {
      edit(name, event.target.value)
    },
  })
  return (
    <fieldset className="cp-doc-roots__fields">
      <legend>Корень {index + 1}</legend>
      <label>
        Репозиторий (ID)
        <input
          {...field('area_id')}
          required
          pattern={technicalIdPattern}
          list="documentation-repositories"
        />
      </label>
      <label>
        Путь в репозитории
        <input
          {...field('path_prefix')}
          maxLength={1024}
          placeholder="Пусто для всего репозитория"
        />
      </label>
      <label>
        Профиль проверки
        <select {...field('profile_id')} required>
          <option value="">Выберите профиль</option>
          {fields.profile_id && fields.profile_id !== gitProfile && (
            <option disabled value={fields.profile_id}>
              {fields.profile_id}
            </option>
          )}
          <option value={gitProfile}>{gitProfile}</option>
        </select>
      </label>
      <label>
        Владелец (ID)
        <input
          {...field('owner_person_id')}
          pattern={technicalIdPattern}
          list="documentation-owners"
          placeholder={noOwner}
        />
        {!fields.owner_person_id && <span>{noOwner}</span>}
      </label>
      <label>
        Аудитория
        <select {...field('audience')} required>
          <option value="">Выберите аудиторию</option>
          {Object.entries(audiences).map(([value, label]) => (
            <option key={value} value={value}>
              {label}
            </option>
          ))}
        </select>
      </label>
      <div>
        <Button
          type="button"
          size="l"
          aria-label={`Удалить корень ${String(index + 1)}`}
          onClick={remove}
        >
          Удалить
        </Button>
      </div>
    </fieldset>
  )
}
