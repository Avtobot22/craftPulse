import { Button } from '@gravity-ui/uikit'
import { useId, useState } from 'react'
import type { CheckProfileOptions } from '@/shared/api'
import { analysisKinds, catalogProfiles, rowKey } from '../model/matrix'
import type { ProfileRow } from '../model/matrix'

export function InitialRowPicker({
  options,
  entries,
  onAdd,
}: {
  options: CheckProfileOptions
  entries: readonly ProfileRow[]
  onAdd: (row: ProfileRow) => void
}) {
  const [selectedComponent, setSelectedComponent] = useState('')
  const [inputLanguage, setInputLanguage] = useState('')
  const [selectedKind, setSelectedKind] =
    useState<ProfileRow['analysis_kind']>()
  const languageId = useId()
  const component =
    options.components.find(({ id }) => id === selectedComponent) ??
    options.components[0]
  if (!component) return <p>У проекта пока нет подтверждённых компонентов.</p>
  if (!options.profiles.length)
    return <p>В каталоге пока нет профилей проверок.</p>

  const component_id = component.id
  const language = inputLanguage.trim()
  const profilesFor = catalogProfiles(options)
  const languages = [
    ...new Set(options.profiles.flatMap(({ languages }) => languages)),
  ]
  const existing = new Set(entries.map(rowKey))
  const available = analysisKinds.flatMap((analysis_kind) => {
    const scope = { component_id, language, analysis_kind }
    if (existing.has(rowKey(scope))) return []
    const builtin = profilesFor(scope, 'builtin')[0]
    const profile_id = builtin ?? profilesFor(scope, 'import')[0]
    if (!profile_id) return []
    const row: ProfileRow = {
      ...scope,
      profile_id,
      mode: builtin ? 'builtin' : 'import',
    }
    return [row]
  })
  const row =
    available.find(({ analysis_kind }) => analysis_kind === selectedKind) ??
    available[0]
  return (
    <fieldset className="cp-check-matrix__row-picker">
      <legend>Новая строка матрицы</legend>
      <label>
        Компонент
        <select
          value={component_id}
          onChange={(event) => {
            setSelectedComponent(event.currentTarget.value)
          }}
        >
          {options.components.map(({ id, path_prefix }) => (
            <option value={id} key={id}>
              {path_prefix || 'Корень репозитория'} · {id}
            </option>
          ))}
        </select>
      </label>
      <label>
        Язык
        <input
          value={inputLanguage}
          onChange={(event) => {
            setInputLanguage(event.currentTarget.value)
          }}
          maxLength={32}
          list={languageId}
          placeholder="Без языка"
        />
      </label>
      <datalist id={languageId}>
        {languages.map((value) => (
          <option value={value} key={value} />
        ))}
      </datalist>
      <label>
        Вид анализа
        <select
          value={row?.analysis_kind ?? ''}
          onChange={(event) => {
            const value = event.currentTarget.value
            setSelectedKind(analysisKinds.find((kind) => kind === value))
          }}
          disabled={!row}
        >
          {!row && <option value="">Нет доступных видов</option>}
          {available.map(({ analysis_kind }) => (
            <option value={analysis_kind} key={analysis_kind}>
              {analysis_kind}
            </option>
          ))}
        </select>
      </label>
      <Button
        type="button"
        disabled={!row}
        onClick={() => {
          if (row) onAdd(row)
        }}
      >
        Добавить строку
      </Button>
    </fieldset>
  )
}
