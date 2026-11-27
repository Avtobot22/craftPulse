import { useState } from 'react'
import { isStrongETag } from '@/shared/api'
import type { ApiClient, ApiResponse } from '@/shared/api'
import type {
  ChangeAttempt,
  ChangeProposal,
  ChangeRecheck,
  ChangeValidation,
  RecheckReason,
} from '@/shared/ui'
import { getStructure, replaceStructure } from '../api/structure'
import type { Structure } from '../api/structure'
import {
  compare,
  fromStructure,
  rebase,
  reconcileCreations,
  toInput,
} from './structure-draft'
import type { Draft } from './structure-draft'
import { serverIssues, validate } from './structure-errors'
import type { Issue } from './structure-errors'

interface RevisionDraft {
  base: Draft
  edited: Draft
}
type Snapshot = ApiResponse<Structure>

export function useStructureDraft(
  api: ApiClient,
  initial: Snapshot,
  externalName: (id: string) => string,
  onSaved: (result: Snapshot) => void,
) {
  const [snapshot, setSnapshot] = useState(initial)
  const [base, setBase] = useState(() => fromStructure(initial.data))
  const [draft, setDraft] = useState(base)
  const [issues, setIssues] = useState<Issue[]>([])
  const [proposal, setProposal] = useState<ChangeProposal<RevisionDraft>>()
  const changesBetween = (before: Draft, after: Draft) => {
    const names = new Map(
      [
        ...before.teams,
        ...before.projects,
        ...before.services,
        ...after.teams,
        ...after.projects,
        ...after.services,
      ].map(({ id, name }) => [id, name]),
    )
    return compare(before, after, (id) => names.get(id) ?? externalName(id))
  }
  const changes = changesBetween(base, draft)

  function edit(next: Draft) {
    setDraft(next)
    setIssues([])
  }

  function openConfirmation() {
    const errors = validate(draft, base)
    setIssues(errors)
    if (errors.length || !changes.length) return
    setProposal({
      etag: snapshot.etag ?? '',
      changes,
      draft: { base, edited: draft },
    })
  }

  function accept(result: Snapshot) {
    const next = fromStructure(result.data)
    setSnapshot(result)
    setBase(next)
    edit(next)
    onSaved(result)
  }

  async function recheck(
    attempt: ChangeAttempt<RevisionDraft>,
    reason: RecheckReason,
    signal: AbortSignal,
  ): Promise<ChangeRecheck<RevisionDraft, Snapshot>> {
    const latest = await getStructure(api, signal)
    signal.throwIfAborted()
    if (!isStrongETag(latest.etag)) return { kind: 'unknown' }
    if (latest.etag === attempt.etag) {
      // An identical conditional replacement cannot overwrite a newer revision.
      return reason === 'uncertain'
        ? { kind: 'retryable' }
        : { kind: 'unknown' }
    }
    const fresh = fromStructure(latest.data)
    const savedChanges = changesBetween(attempt.draft.base, fresh)
    const local =
      reason === 'uncertain'
        ? reconcileCreations(attempt.draft.base, attempt.draft.edited, fresh)
        : attempt.draft.edited
    const { draft: edited, hasConflictingProposalDecision } = rebase(
      attempt.draft.base,
      local,
      fresh,
    )
    const remaining = changesBetween(fresh, edited)
    if (
      reason === 'uncertain' &&
      !remaining.length &&
      !hasConflictingProposalDecision
    ) {
      return { kind: 'applied', result: latest }
    }
    setSnapshot(latest)
    setBase(fresh)
    edit(edited)
    return {
      kind: 'not-applied',
      proposal: {
        etag: latest.etag,
        referenceChanges: savedChanges.map((change) => ({
          ...change,
          label: 'Уже сохранено · ' + change.label,
        })),
        changes: remaining,
        draft: { base: fresh, edited },
      },
    }
  }

  return {
    snapshot,
    draft,
    issues,
    proposal,
    dirty: changes.length > 0,
    edit,
    openConfirmation,
    discard: () => {
      edit(base)
    },
    closeConfirmation: () => {
      setProposal(undefined)
    },
    submit: (attempt: ChangeAttempt<RevisionDraft>, signal: AbortSignal) =>
      replaceStructure(
        api,
        toInput(attempt.draft.edited),
        attempt.etag,
        signal,
      ),
    recheck,
    accept,
    onValidation: (result: ChangeValidation) => {
      setIssues(serverIssues(result, draft))
    },
  }
}
