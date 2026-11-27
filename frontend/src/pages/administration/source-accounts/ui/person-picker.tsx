import { Button } from '@gravity-ui/uikit'
import { useState } from 'react'
import type { Person } from '../model/assignments'

export function PersonPicker({
  people,
  disabled,
  onChoose,
}: {
  people: ReadonlyMap<string, Person>
  disabled: boolean
  onChoose: (id: string) => void
}) {
  const [search, setSearch] = useState('')
  const [selected, setSelected] = useState('')
  const needle = search.trim().toLocaleLowerCase('ru')
  const matches = [...people.values()].filter(
    (person) =>
      person.kind === 'human' &&
      (person.display_name ?? person.id)
        .toLocaleLowerCase('ru')
        .includes(needle),
  )
  const value = matches.some(({ id }) => id === selected) ? selected : ''

  const names = new Map<string, number>()
  for (const { display_name } of matches) {
    if (display_name)
      names.set(display_name, (names.get(display_name) ?? 0) + 1)
  }

  return (
    <div className="cp-account-fields">
      <label>
        Поиск человека
        <input
          type="search"
          value={search}
          disabled={disabled}
          onChange={(event) => {
            setSearch(event.target.value)
          }}
        />
      </label>
      <label>
        Человек
        <select
          value={value}
          disabled={disabled || !matches.length}
          onChange={(event) => {
            setSelected(event.target.value)
          }}
        >
          <option value="">
            {matches.length ? 'Выберите человека' : 'Никого не найдено'}
          </option>
          {matches.map(({ id, display_name }) => (
            <option key={id} value={id}>
              {display_name && (names.get(display_name) ?? 0) > 1
                ? `${display_name} (${id})`
                : (display_name ?? id)}
            </option>
          ))}
        </select>
      </label>
      <Button
        size="xl"
        disabled={disabled || !value}
        onClick={() => {
          onChoose(value)
        }}
      >
        Выбрать человека
      </Button>
    </div>
  )
}
