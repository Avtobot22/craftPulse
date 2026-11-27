import { useQuery } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import { Link } from 'react-router'
import { isUnavailableError } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { routePaths } from '@/shared/config'
import {
  CursorPagination,
  EmptyState,
  ErrorState,
  LoadingState,
  UnavailableRedirect,
} from '@/shared/ui'
import { usersPage } from '../api/users-api'
import type { User, UserParams } from '../api/users-api'

type Sort = NonNullable<UserParams['sort']>
type Filter = NonNullable<UserParams['status']> | ''
const sortLabels: Record<Sort, string> = {
  display_name: 'По имени, А–Я',
  '-display_name': 'По имени, Я–А',
  created_at: 'Сначала добавленные раньше',
  '-created_at': 'Сначала добавленные недавно',
}
const sortOrders = Object.keys(sortLabels).filter((key): key is Sort =>
  Object.hasOwn(sortLabels, key),
)

export function userName(user: User) {
  const name = user.display_name?.trim()
  if (!name) return 'Имя не получено'
  return name
}

export function userStatus(status: User['status']) {
  switch (status) {
    case 'active':
      return 'Активен'
    case 'blocked':
      return 'Заблокирован'
    case 'unknown':
      return 'Неизвестный статус'
  }
}

export function UserList({
  api,
  selected,
}: {
  api: ApiClient
  selected: string | undefined
}) {
  const heading = useRef<HTMLHeadingElement>(null)
  const [status, setStatus] = useState<Filter>('')
  const [sort, setSort] = useState<Sort>('display_name')
  const [paging, setPaging] = useState({
    cursor: '',
    previous: [] as string[],
    limit: 25,
  })
  const { cursor, previous, limit } = paging
  const query = useQuery(
    usersPage(api, {
      limit,
      sort,
      ...(status ? { status } : {}),
      ...(cursor ? { cursor } : {}),
    }),
  )
  const page = query.data
  if (isUnavailableError(query.error))
    return <UnavailableRedirect error={query.error} />

  function goTo(cursor: string, previous: string[], limit: string) {
    setPaging({ cursor, previous, limit: Number(limit) })
    heading.current?.focus()
  }

  return (
    <section aria-labelledby="users-list-title" className="cp-users__panel">
      <h2 id="users-list-title" ref={heading} tabIndex={-1}>
        Список пользователей
      </h2>
      <div className="cp-users__controls">
        <label>
          Статус
          <select
            value={status}
            onChange={({ target: { value } }) => {
              if (value !== '' && value !== 'active' && value !== 'blocked')
                return
              setStatus(value)
              setPaging({ cursor: '', previous: [], limit })
            }}
          >
            <option value="">Все статусы</option>
            <option value="active">Активен</option>
            <option value="blocked">Заблокирован</option>
          </select>
        </label>
        <label>
          Сортировка
          <select
            value={sort}
            onChange={({ target: { value } }) => {
              const order = sortOrders.find((item) => item === value)
              if (!order) return
              setSort(order)
              setPaging({ cursor: '', previous: [], limit })
            }}
          >
            {sortOrders.map((order) => (
              <option key={order} value={order}>
                {sortLabels[order]}
              </option>
            ))}
          </select>
        </label>
      </div>
      {query.isPending && <LoadingState />}
      {query.isError && (
        <ErrorState
          reason="Не удалось загрузить пользователей"
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      )}
      {page &&
        !query.isError &&
        (page.data.length === 0 ? (
          <EmptyState reason="filters" title="Пользователи не найдены" />
        ) : (
          <table className="cp-users__table" aria-labelledby="users-list-title">
            <thead>
              <tr>
                <th scope="col">Имя</th>
                <th scope="col">Статус</th>
              </tr>
            </thead>
            <tbody>
              {page.data.map((user) => (
                <tr
                  key={user.id}
                  data-selected={selected === user.id || undefined}
                >
                  <td>
                    <Link
                      to={routePaths.users + '/' + encodeURIComponent(user.id)}
                      aria-current={selected === user.id ? 'page' : undefined}
                    >
                      {userName(user)}
                    </Link>
                  </td>
                  <td>{userStatus(user.status)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        ))}
      <CursorPagination
        label="Страницы пользователей"
        cursor={cursor}
        previous={previous}
        next={
          query.isError || query.isFetching ? null : (page?.next_cursor ?? null)
        }
        received={query.isError ? undefined : page?.data.length}
        limit={limit}
        goTo={goTo}
      />
    </section>
  )
}
