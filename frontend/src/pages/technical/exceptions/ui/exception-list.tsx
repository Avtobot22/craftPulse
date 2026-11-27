import { Button } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { generatePath, Link, useSearchParams } from 'react-router'
import { RestrictedResults } from '@/entities/result-access'
import {
  describeExceptionScope,
  exceptionOrigin,
  exceptionStatus,
  formatExceptionDate,
} from '@/entities/exception'
import { listExceptions } from '@/shared/api'
import type { ApiClient, ExceptionListParams } from '@/shared/api'
import { contractLabel, routePaths, technicalLabels } from '@/shared/config'
import {
  ErrorState,
  InspectionLink,
  LoadingState,
  useInspectionReturn,
} from '@/shared/ui'
import {
  readExceptionSearch,
  sorts,
  states,
  uuid,
} from '../model/exception-search'

export function ExceptionList({ api }: { api: ApiClient }) {
  const [search, setSearch] = useSearchParams()
  const filters = readExceptionSearch(search)
  const page = useQuery(listExceptions(api, filters))
  useInspectionReturn(page.isSuccess)
  const origin = exceptionOrigin(search, filters.finding_id)

  function goTo(cursor: string | null) {
    const next = new URLSearchParams(search)
    if (cursor) next.set('cursor', cursor)
    else next.delete('cursor')
    setSearch(next)
  }

  return (
    <>
      <ExceptionFilters
        key={JSON.stringify(filters)}
        filters={filters}
        onFilter={(values) => {
          const next = new URLSearchParams(search)
          next.delete('cursor')
          for (const [key, value] of Object.entries(values)) {
            if (value) next.set(key, value)
            else next.delete(key)
          }
          setSearch(next)
        }}
      />
      {page.isPending && <LoadingState />}
      {page.isError && (
        <ErrorState
          reason="Не удалось загрузить список исключений"
          onRetry={() => void page.refetch()}
          retrying={page.isFetching}
        />
      )}
      {page.isSuccess && (
        <>
          <RestrictedResults
            access_notes={page.data.access_notes}
            access_limited={page.data.access_limited}
          />
          {page.data.data.length === 0 && (
            <p>Исключений по этим фильтрам нет.</p>
          )}
          <ul className="cp-exceptions__list">
            {page.data.data.map(({ id, state, current_version: version }) => (
              <li key={id}>
                <InspectionLink
                  id={`exception-${id}`}
                  to={{
                    pathname: generatePath(routePaths.exceptions, {
                      exceptionId: id,
                    }),
                    search: search.toString(),
                  }}
                >
                  {contractLabel(
                    technicalLabels.exceptionDecisionType,
                    version.decision_type,
                  )}{' '}
                  · {version.reason}
                </InspectionLink>
                <p>{exceptionStatus(state)}</p>
                {version.status === 'pending' && state === 'active' && (
                  <p>Новая версия ждёт подтверждения ревьюером.</p>
                )}
                <p>{describeExceptionScope(version.scope)}</p>
                <p>
                  {state === 'expired' ? 'Срок истёк ' : 'Срок пересмотра '}
                  <time dateTime={version.expires_at}>
                    {formatExceptionDate(version.expires_at)}
                  </time>
                </p>
              </li>
            ))}
          </ul>
          {Boolean(filters.cursor ?? page.data.next_cursor) && (
            <nav
              className="cp-exceptions__actions"
              aria-label="Страницы истории"
            >
              {filters.cursor && (
                <Button
                  onClick={() => {
                    goTo(null)
                  }}
                >
                  К первой странице
                </Button>
              )}
              {page.data.next_cursor &&
                page.data.next_cursor !== filters.cursor && (
                  <Button
                    onClick={() => {
                      goTo(page.data.next_cursor)
                    }}
                  >
                    Следующая страница
                  </Button>
                )}
            </nav>
          )}
        </>
      )}
      {origin && <Link to={origin}>К исходной находке</Link>}
    </>
  )
}

function ExceptionFilters({
  filters,
  onFilter,
}: {
  filters: ExceptionListParams
  onFilter: (values: Record<string, string>) => void
}) {
  const [state, setState] = useState(filters.state ?? '')
  const [component, setComponent] = useState(filters.component_id ?? '')
  const [finding, setFinding] = useState(filters.finding_id ?? '')
  const [sort, setSort] = useState<string>(filters.sort ?? 'expires_at')

  return (
    <form
      className="cp-exceptions__filters"
      onSubmit={(event) => {
        event.preventDefault()
        onFilter({
          state,
          component_id: component.trim(),
          finding_id: finding.trim(),
          sort,
        })
      }}
    >
      <label>
        Состояние
        <select
          value={state}
          onChange={(event) => {
            setState(event.target.value)
          }}
        >
          <option value="">Все</option>
          {states.map((value) => (
            <option key={value} value={value}>
              {exceptionStatus(value)}
            </option>
          ))}
        </select>
      </label>
      <label>
        ID компонента
        <input
          value={component}
          pattern={uuid.source.replace('a-f', 'a-fA-F')}
          onChange={(event) => {
            setComponent(event.target.value)
          }}
        />
      </label>
      <label>
        ID находки
        <input
          value={finding}
          pattern={uuid.source.replace('a-f', 'a-fA-F')}
          onChange={(event) => {
            setFinding(event.target.value)
          }}
        />
      </label>
      <label>
        Порядок
        <select
          value={sort}
          onChange={(event) => {
            setSort(event.target.value)
          }}
        >
          {Object.entries(sorts).map(([value, label]) => (
            <option key={value} value={value}>
              {label}
            </option>
          ))}
        </select>
      </label>
      <Button type="submit" view="outlined">
        Применить фильтры
      </Button>
    </form>
  )
}
