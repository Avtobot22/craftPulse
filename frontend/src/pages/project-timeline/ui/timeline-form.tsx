import { Button, Label, Text } from '@gravity-ui/uikit'
import { useState } from 'react'
import {
  actors,
  dayWindow,
  intervalWindow,
  objects,
  pageSizes,
  uuid,
} from '../model/timeline-search'
import type { TimelineSearch } from '../model/timeline-search'

export function TimelineForm({
  draft,
  issue,
  apply,
}: Pick<TimelineSearch, 'draft' | 'issue'> & {
  apply: (values: Record<string, string>) => void
}) {
  const [mode, setMode] = useState<'day' | 'interval'>(draft.mode)
  const [error, setError] = useState(issue)
  const actor = draft.actor && actors[draft.actor]
  const object = draft.object && objects[draft.object]
  const appliedRevision = uuid.test(draft.revision) ? draft.revision : ''
  const filterCount = [actor, object, appliedRevision].filter(Boolean).length

  return (
    <form
      className="cp-timeline__controls"
      aria-label="Период и фильтры"
      noValidate
      onSubmit={(event) => {
        event.preventDefault()
        const form = new FormData(event.currentTarget)
        const get = (name: string) => {
          const value = form.get(name)
          return typeof value === 'string' ? value.trim() : ''
        }
        const date = get('date')
        const period =
          mode === 'day'
            ? dayWindow(date)
            : intervalWindow(get('from') + 'Z', get('to') + 'Z')
        if (!period) {
          setError(
            mode === 'day'
              ? 'Укажите существующую дату.'
              : 'Укажите начало и конец интервала. Конец должен быть позже начала.',
          )
          return
        }
        const reset =
          event.nativeEvent instanceof SubmitEvent &&
          event.nativeEvent.submitter?.getAttribute('value') === 'reset'
        const revision = reset ? '' : get('revision_id').toLowerCase()
        if (revision && !uuid.test(revision)) {
          setError('Укажите UUID ревизии.')
          return
        }
        setError('')
        apply({
          date: mode === 'day' ? date : '',
          from: mode === 'interval' ? period.from : '',
          to: mode === 'interval' ? period.to : '',
          actor_kind: reset ? '' : get('actor_kind'),
          object_kind: reset ? '' : get('object_kind'),
          revision_id: revision,
          limit: get('limit') === '25' ? '' : get('limit'),
        })
        if (reset) {
          for (const name of ['actor_kind', 'object_kind', 'revision_id']) {
            const input = event.currentTarget.elements.namedItem(name)
            if (
              input instanceof HTMLInputElement ||
              input instanceof HTMLSelectElement
            )
              input.value = ''
          }
        }
      }}
    >
      <Text as="p" color="secondary">
        Часовой пояс таймлайна: UTC
      </Text>
      <div className="cp-timeline__fields">
        <label>
          Период
          <select
            value={mode}
            onChange={(event) => {
              setMode(event.target.value === 'interval' ? 'interval' : 'day')
            }}
          >
            <option value="day">Дата</option>
            <option value="interval">Интервал</option>
          </select>
        </label>
        {mode === 'day' ? (
          <label>
            Дата
            <input type="date" name="date" defaultValue={draft.date} required />
          </label>
        ) : (
          <>
            <label>
              С (включительно)
              <input
                type="datetime-local"
                name="from"
                step="any"
                defaultValue={
                  draft.from || (draft.date && draft.date + 'T00:00')
                }
                required
              />
            </label>
            <label>
              До (исключительно)
              <input
                type="datetime-local"
                name="to"
                step="any"
                defaultValue={
                  draft.to || dayWindow(draft.date)?.to.slice(0, -1)
                }
                required
              />
            </label>
          </>
        )}
      </div>
      <details open>
        <summary>
          Фильтры{filterCount > 0 && ` (${String(filterCount)})`}
        </summary>
        <div className="cp-timeline__fields">
          <label>
            Вид участника
            <select name="actor_kind" defaultValue={draft.actor}>
              <option value="">Все участники</option>
              {Object.entries(actors).map(([value, name]) => (
                <option key={value} value={value}>
                  {name}
                </option>
              ))}
            </select>
          </label>
          <label>
            Вид объекта
            <select name="object_kind" defaultValue={draft.object}>
              <option value="">Все объекты</option>
              {Object.entries(objects).map(([value, name]) => (
                <option key={value} value={value}>
                  {name}
                </option>
              ))}
            </select>
          </label>
          <label>
            Ревизия (UUID)
            <input
              type="text"
              name="revision_id"
              defaultValue={draft.revision}
            />
          </label>
          <label>
            Событий за запрос
            <select name="limit" defaultValue={draft.limit}>
              {pageSizes.map((size) => (
                <option key={size} value={size}>
                  {size}
                </option>
              ))}
            </select>
          </label>
        </div>
      </details>
      <div className="cp-timeline__fields">
        <Button size="xl" view="action" type="submit">
          Применить
        </Button>
        <Button size="xl" type="submit" value="reset">
          Сбросить фильтры
        </Button>
      </div>
      {error && (
        <Text as="p" role="alert" color="danger">
          {error}
        </Text>
      )}
      <div
        className="cp-timeline__applied"
        role="group"
        aria-label="Применённые фильтры"
      >
        {filterCount > 0 ? (
          <>
            {actor && <Label size="m">{actor}</Label>}
            {object && <Label size="m">{object}</Label>}
            {appliedRevision && (
              <Label size="m">Ревизия: {appliedRevision}</Label>
            )}
          </>
        ) : (
          <Text color="secondary">Фильтры не применены</Text>
        )}
      </div>
    </form>
  )
}
