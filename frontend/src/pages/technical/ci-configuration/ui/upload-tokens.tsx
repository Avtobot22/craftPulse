import { Button } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { listUploadTokens } from '@/shared/api'
import type { ApiClient, UploadToken } from '@/shared/api'
import {
  CursorPagination,
  ErrorState,
  LoadingState,
  StatusTime,
} from '@/shared/ui'
import { UploadTokenAction } from './upload-token-action'
import type { TokenAction } from './upload-token-action'

const firstPage = { cursor: '', previous: [] as string[], limit: 25 }

export function UploadTokens({
  api,
  projectId,
}: {
  api: ApiClient
  projectId: string
}) {
  const [selection, setSelection] = useState<{
    action: TokenAction
    opener: HTMLButtonElement
  }>()
  const [notice, setNotice] = useState('')
  const [page, setPage] = useState(firstPage)
  const { cursor, previous, limit } = page
  const query = useQuery(
    listUploadTokens(api, {
      project_id: projectId,
      limit,
      ...(cursor ? { cursor } : {}),
    }),
  )

  function start(action: TokenAction, opener: HTMLButtonElement) {
    setNotice('')
    setSelection({ action, opener })
  }

  return (
    <section aria-label="Токены загрузки">
      <h2>Токены загрузки</h2>
      {query.isPending && <LoadingState />}
      {query.isError && (
        <ErrorState
          reason="Не удалось загрузить токены проекта. Если доступ отозван, обратитесь к администратору."
          onRetry={() => void query.refetch()}
          retrying={query.isFetching}
        />
      )}
      {!query.isError && query.data && (
        <>
          {query.data.data.length ? (
            <ul className="cp-ci-setup__tokens">
              {query.data.data.map((token) => (
                <TokenRow
                  key={token.id}
                  token={token}
                  onRevoke={(opener) => {
                    start({ kind: 'revoke', token }, opener)
                  }}
                />
              ))}
            </ul>
          ) : (
            <p>На этой странице нет токенов.</p>
          )}
          {(cursor || query.data.next_cursor) && (
            <CursorPagination
              label="Страницы токенов"
              cursor={cursor}
              next={query.data.next_cursor}
              previous={previous}
              limit={limit}
              received={query.data.data.length}
              goTo={(cursor, previous, limit) => {
                setPage({ cursor, previous, limit: Number(limit) })
              }}
            />
          )}
          <Button
            view="action"
            onClick={(event) => {
              start({ kind: 'create' }, event.currentTarget)
            }}
          >
            Выпустить токен
          </Button>
        </>
      )}
      {notice && <p role="alert">{notice}</p>}
      {selection && (
        <UploadTokenAction
          api={api}
          projectId={projectId}
          action={selection.action}
          opener={selection.opener}
          onIssued={() => {
            setPage(firstPage)
          }}
          onClose={() => {
            setSelection(undefined)
          }}
          onNotice={setNotice}
        />
      )}
    </section>
  )
}

function TokenRow({
  token,
  onRevoke,
}: {
  token: UploadToken
  onRevoke: (opener: HTMLButtonElement) => void
}) {
  return (
    <li>
      <strong>Слот {token.slot}</strong> ·{' '}
      {token.revoked_at === null ? 'Действует' : 'Отозван'}
      <dl>
        <dt>Выпущен</dt>
        <dd>
          <StatusTime value={token.created_at} />
        </dd>
        <dt>Последнее использование</dt>
        <dd>
          <StatusTime value={token.last_used_at ?? null} />
        </dd>
        {token.revoked_at !== null && (
          <>
            <dt>Отозван</dt>
            <dd>
              <StatusTime value={token.revoked_at} />
            </dd>
          </>
        )}
      </dl>
      {token.revoked_at === null && (
        <Button
          onClick={(event) => {
            onRevoke(event.currentTarget)
          }}
        >
          Отозвать токен (слот {token.slot})
        </Button>
      )}
    </li>
  )
}
