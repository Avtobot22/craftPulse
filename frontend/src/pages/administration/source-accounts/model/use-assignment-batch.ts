import { useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import type { ApiClient, components } from '@/shared/api'
import { readAffectedAccounts } from '../api/directory'
import type { Account, Decision, Draft, RowResult } from './assignments'
import { batchLimit } from './assignments'

type Receipt = components['schemas']['AccountAssignmentBatchResult']

export function useAssignmentBatch(api: ApiClient) {
  const client = useQueryClient()
  const [drafts, setDrafts] = useState<ReadonlyMap<string, Draft>>(new Map())
  const [results, setResults] = useState<ReadonlyMap<string, RowResult>>(
    new Map(),
  )
  const [lastReceipt, setLastReceipt] = useState<Receipt>()
  const [refreshing, setRefreshing] = useState(false)

  function decide(account: Account, decision: Decision | null) {
    setDrafts((previous) => {
      const next = new Map(previous)
      if (!decision) next.delete(account.id)
      else if (next.has(account.id) || next.size < batchLimit) {
        next.set(account.id, { account, decision })
      }
      return next
    })
  }

  async function refresh(rows: readonly RowResult[]) {
    if (!rows.length) return
    setRefreshing(true)
    try {
      const fresh = await readAffectedAccounts(api, rows)

      setResults((previous) => {
        const next = new Map(previous)
        for (const row of rows) {
          const account = fresh.get(row.account.id)
          next.set(
            row.account.id,
            account
              ? { ...row, account, refresh: 'ready' }
              : { ...row, refresh: 'missing' },
          )
        }
        return next
      })
      setDrafts((previous) => {
        const next = new Map(previous)
        for (const [id, account] of fresh) {
          const draft = next.get(id)
          if (draft) next.set(id, { ...draft, account })
        }
        return next
      })
      void client.invalidateQueries({ queryKey: ['source-accounts', 'list'] })
    } catch {
      // The write receipt is already known. Offer a fresh read, not a new POST.
    } finally {
      setRefreshing(false)
    }
  }

  function receive(receipt: Receipt, submitted: readonly Draft[]) {
    const snapshots = new Map(
      submitted.map(({ account }) => [account.id, account]),
    )
    const rows: RowResult[] = []
    for (const outcome of receipt.items) {
      const account = snapshots.get(outcome.source_account_id)
      if (account)
        rows.push({
          account,
          outcome,
          baseAssignmentId: account.assignment?.id ?? null,
          refresh: outcome.outcome === 'rejected' ? 'ready' : 'needed',
        })
    }

    setLastReceipt(receipt)
    setResults((previous) => {
      const next = new Map(
        [...previous].filter(
          ([id, row]) => row.refresh === 'needed' || drafts.has(id),
        ),
      )
      for (const row of rows) next.set(row.account.id, row)
      return next
    })
    setDrafts((previous) => {
      const next = new Map(previous)
      for (const { account, outcome } of rows) {
        if (outcome.outcome === 'saved' || outcome.outcome === 'unchanged')
          next.delete(account.id)
      }
      return next
    })
    if (
      submitted.some(({ decision }) => decision.kind === 'create') ||
      receipt.items.some(({ error_code }) => error_code === 'person_not_found')
    )
      void client.invalidateQueries({ queryKey: ['persons'] })
    void refresh(rows.filter(({ refresh }) => refresh === 'needed'))
  }

  const pendingReads = [...results.values()].filter(
    ({ refresh }) => refresh === 'needed',
  )

  return {
    drafts,
    results,
    lastReceipt,
    refreshing,
    pendingReads,
    decide,
    receive,
    retryRead: () => void refresh(pendingReads),
    discard: () => {
      setDrafts(new Map())
    },
  }
}
