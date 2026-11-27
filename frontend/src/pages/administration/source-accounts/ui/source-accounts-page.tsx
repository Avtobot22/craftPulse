import { Button, Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import {
  ApiError,
  createAccountAssignments,
  listSourceAccounts,
} from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import {
  ChangeConfirmationDialog,
  CursorPagination,
  EmptyState,
  ErrorState,
  LoadingState,
  UnavailableRedirect,
  UnsavedChangesDialog,
} from '@/shared/ui'
import type { ChangeProposal } from '@/shared/ui'
import { instanceDirectory, peopleDirectory } from '../api/directory'
import {
  accountLabel,
  assignmentItem,
  assignmentLabel,
  batchLimit,
  decisionLabel,
} from '../model/assignments'
import type { Draft } from '../model/assignments'
import { useAccountFilters } from '../model/use-account-filters'
import { useAssignmentBatch } from '../model/use-assignment-batch'
import { AccountsTable } from './accounts-table'
import { BatchResults } from './batch-results'
import './source-accounts-page.css'

export function SourceAccountsPage({ api }: { api: ApiClient }) {
  const filters = useAccountFilters()
  const accountsQuery = useQuery(listSourceAccounts(api, filters.params))
  const peopleQuery = useQuery(peopleDirectory(api))
  const instancesQuery = useQuery(instanceDirectory(api))
  const people = new Map(peopleQuery.data?.map((person) => [person.id, person]))
  const batch = useAssignmentBatch(api)
  const [proposal, setProposal] = useState<ChangeProposal<Draft[]> | null>(null)
  const [writeError, setWriteError] = useState(false)
  const heading = useRef<HTMLHeadingElement>(null)
  const opener = useRef<HTMLButtonElement>(null)
  const form = useRef<HTMLFormElement>(null)
  const error =
    accountsQuery.error instanceof ApiError ? accountsQuery.error : null
  if (error?.kind === 'forbidden')
    return <UnavailableRedirect error={error} forbidden />

  const accounts = (accountsQuery.data?.data ?? []).map((account) => {
    const result = batch.results.get(account.id)
    return result?.refresh === 'ready' &&
      result.outcome.outcome !== 'rejected' &&
      result.baseAssignmentId === (account.assignment?.id ?? null) &&
      (accountsQuery.isFetching || accountsQuery.isError)
      ? result.account
      : account
  })
  const visibleIds = new Set(accounts.map(({ id }) => id))
  const outside = new Map(
    [...batch.results.values(), ...batch.drafts.values()]
      .filter(({ account }) => !visibleIds.has(account.id))
      .map(({ account }) => [account.id, account]),
  )

  const reviewable = [...batch.drafts.values()].filter(({ account }) => {
    const result = batch.results.get(account.id)
    return !result || result.refresh === 'ready'
  })

  function inspect() {
    setWriteError(false)
    setProposal({
      // The API compares an assignment ID for each item instead of an ETag.
      etag: '',
      draft: reviewable,
      changes: reviewable.map((draft) => ({
        label: accountLabel(draft.account),
        before: assignmentLabel(draft.account.assignment, people),
        after: decisionLabel(draft, people),
      })),
    })
  }

  return (
    <section className="cp-accounts" aria-labelledby="accounts-heading">
      <Text
        as="h1"
        id="accounts-heading"
        variant="header-1"
        ref={heading}
        tabIndex={-1}
      >
        Учётные записи
      </Text>
      <form
        ref={form}
        className="cp-accounts"
        onSubmit={(event) => {
          event.preventDefault()
        }}
      >
        <div className="cp-accounts__bar">
          <label>
            Статус привязки
            <select
              value={filters.assignment ?? ''}
              onChange={(event) => {
                filters.update({ nextAssignment: event.target.value })
              }}
            >
              <option value="">Все учётные записи</option>
              <option value="assigned">Сопоставлены</option>
              <option value="unassigned">Не сопоставлены</option>
            </select>
          </label>
          <label>
            Экземпляр источника
            <select
              value={filters.instance}
              onChange={(event) => {
                filters.update({ nextInstance: event.target.value })
              }}
            >
              <option value="">Все экземпляры</option>
              {filters.instance &&
                !instancesQuery.data?.some(
                  ({ id }) => id === filters.instance,
                ) && (
                  <option value={filters.instance}>{filters.instance}</option>
                )}
              {instancesQuery.data?.map(({ id, label }) => (
                <option key={id} value={id}>
                  {label}
                </option>
              ))}
            </select>
          </label>
        </div>
        {instancesQuery.isError && (
          <ErrorState
            reason="Не удалось загрузить экземпляры источников"
            retrying={instancesQuery.isFetching}
            onRetry={() => void instancesQuery.refetch()}
          />
        )}
        {peopleQuery.isError && (
          <ErrorState
            reason="Не удалось загрузить список людей"
            retrying={peopleQuery.isFetching}
            onRetry={() => void peopleQuery.refetch()}
          />
        )}
        <div className="cp-accounts__bar">
          <Button
            ref={opener}
            view="action"
            size="xl"
            disabled={batch.refreshing || reviewable.length === 0}
            onClick={inspect}
          >
            Просмотреть выбранные
          </Button>
          <span role="status">
            Выбрано решений: {batch.drafts.size} из {batchLimit}
          </span>
        </div>
        {writeError && (
          <p role="alert">
            Пакет отклонён. Проверьте решения перед новым подтверждением.
          </p>
        )}
        <BatchResults
          receipt={batch.lastReceipt}
          results={batch.results}
          refreshing={batch.refreshing}
          needsRefresh={batch.pendingReads.length > 0}
          onRetry={batch.retryRead}
        />
        {accountsQuery.isPending && <LoadingState />}
        {accountsQuery.isError && (
          <ErrorState
            reason="Не удалось загрузить учётные записи"
            {...(error?.problem?.correlation_id
              ? { correlationId: error.problem.correlation_id }
              : {})}
            retrying={accountsQuery.isFetching}
            onRetry={() => void accountsQuery.refetch()}
          />
        )}
        {accounts.length > 0 && (
          <AccountsTable
            accounts={accounts}
            label="Учётные записи"
            state={{ ...batch, people }}
          />
        )}
        {accountsQuery.isSuccess && !accounts.length && (
          <EmptyState
            reason={
              filters.assignment || filters.instance || filters.cursor
                ? 'filters'
                : 'source'
            }
            title={
              filters.assignment || filters.instance || filters.cursor
                ? 'Нет учётных записей по фильтру'
                : 'Нет учётных записей'
            }
          />
        )}
        {(accounts.length > 0 ||
          Boolean(accountsQuery.data?.next_cursor) ||
          filters.cursor) && (
          <CursorPagination
            label="Страницы учётных записей"
            cursor={filters.cursor}
            previous={filters.previous}
            next={accountsQuery.data?.next_cursor ?? null}
            limit={filters.limit}
            received={accountsQuery.data?.data.length}
            goTo={(nextCursor, nextHistory, nextLimit) => {
              heading.current?.focus()
              filters.update({ nextCursor, nextHistory, nextLimit })
            }}
          />
        )}
        {outside.size > 0 && (
          <AccountsTable
            accounts={[...outside.values()]}
            label="Выбранные решения и результаты вне страницы"
            state={{ ...batch, people }}
          />
        )}
      </form>
      {proposal && (
        <ChangeConfirmationDialog
          title="Подтвердите решения по учётным записям"
          scope="Учётные записи источников"
          consequences="Смена признака бота пересчитывает затронутые показатели новой ревизией."
          initialWrite
          wide
          openerRef={opener}
          proposal={proposal}
          submit={({ draft, idempotencyKey }, signal) =>
            createAccountAssignments(
              api,
              { items: draft.map(assignmentItem) },
              idempotencyKey,
              signal,
            )
          }
          recheck={(_attempt, reason) =>
            Promise.resolve(
              reason === 'uncertain'
                ? { kind: 'retryable' }
                : { kind: 'unknown' },
            )
          }
          onSaved={(result) => {
            batch.receive(result, proposal.draft)
          }}
          onValidation={() => {
            setWriteError(true)
          }}
          onClose={() => {
            setProposal(null)
          }}
        />
      )}
      <UnsavedChangesDialog
        formName="Решения по учётным записям"
        formRef={form}
        dirty={batch.drafts.size > 0}
        confirmationOpen={proposal !== null}
        allowSearchChanges
        onDiscard={batch.discard}
      />
    </section>
  )
}
