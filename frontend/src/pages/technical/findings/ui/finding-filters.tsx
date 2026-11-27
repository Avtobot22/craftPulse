import { Button } from '@gravity-ui/uikit'
import type { SubmitEvent } from 'react'
import type { FindingListParams } from '@/shared/api'
import { technicalLabels } from '@/shared/config'
import {
  findingSorts,
  idPattern,
  scopeFilterKeys,
  scopeFilters,
} from '../model/finding-search'

const filters = [
  { name: 'finding_kind', label: 'Вид', options: technicalLabels.findingKind },
  { name: 'state', label: 'Состояние', options: technicalLabels.findingState },
  { name: 'severity', label: 'Серьёзность', options: technicalLabels.severity },
  { name: 'sort', label: 'Сортировка', options: findingSorts },
] as const

export function FindingFilters({
  params,
  apply,
}: {
  params: FindingListParams
  apply: (values: URLSearchParams) => void
}) {
  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    const next = new URLSearchParams()
    for (const [key, value] of new FormData(event.currentTarget)) {
      if (typeof value === 'string' && value) next.set(key, value)
    }
    apply(next)
  }

  return (
    <form
      className="cp-findings__filters"
      aria-label="Фильтры находок"
      onSubmit={submit}
    >
      {filters.map(({ name, label, options }) => (
        <label key={name}>
          {label}
          <select
            name={name}
            defaultValue={params[name] ?? (name === 'sort' ? '-severity' : '')}
          >
            {name !== 'sort' && <option value="">Все</option>}
            {Object.entries(options).map(([code, title]) => (
              <option key={code} value={code}>
                {title}
              </option>
            ))}
          </select>
        </label>
      ))}
      {scopeFilterKeys.map((name) => (
        <label key={name}>
          {scopeFilters[name]}
          <input
            name={name}
            pattern={idPattern}
            placeholder="ID (UUID)"
            defaultValue={params[name] ?? ''}
          />
        </label>
      ))}
      <Button type="submit" size="xl" view="action">
        Применить
      </Button>
    </form>
  )
}
