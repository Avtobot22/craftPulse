import { useLayoutEffect, useRef, useState } from 'react'
import { ApiError, fieldErrorsByPointer, isStrongETag } from '@/shared/api'
import type {
  ChangeAttempt,
  ChangeConfirmationDialogProps,
  ChangeProposal,
  RecheckReason,
  ReconciledChange,
} from './change-confirmation'

// A lost write response starts reconciliation with a fresh signal.
const requestTimeoutMs = 30_000

type Notice = 'updated' | 'not-applied'

type Status<Draft, Result> =
  | { kind: 'preview'; proposal: ChangeProposal<Draft>; notice?: Notice }
  | { kind: 'saving'; attempt: ChangeAttempt<Draft> }
  | { kind: 'retryable'; attempt: ChangeAttempt<Draft> }
  | {
      kind: 'checking' | 'check-failed'
      attempt: ChangeAttempt<Draft>
      reason: RecheckReason
    }
  | { kind: 'rejected'; attempt: ChangeAttempt<Draft> }
  | {
      kind: 'reconciled'
      attempt: ChangeAttempt<Draft>
      outcome: ReconciledChange<Result>
    }

export function useChangeConfirmation<Draft, Result>({
  proposal: initialProposal,
  initialWrite = false,
  onRejected,
  submit,
  recheck,
  onSaved,
  onValidation,
  onClose,
}: ChangeConfirmationDialogProps<Draft, Result>) {
  const [status, setStatus] = useState<Status<Draft, Result>>({
    kind: 'preview',
    proposal: initialProposal,
  })
  const active = useRef(false)
  const busy = useRef(false)
  useLayoutEffect(() => {
    active.current = true
    return () => {
      active.current = false
    }
  }, [])

  const working = status.kind === 'saving' || status.kind === 'checking'
  const uncertain =
    status.kind === 'check-failed' && status.reason === 'uncertain'
  const proposal = status.kind === 'preview' ? status.proposal : status.attempt

  // Release the lock after the new UI commits so an old handler cannot resubmit.
  useLayoutEffect(() => {
    if (!working) busy.current = false
  }, [status, working])

  function close() {
    active.current = false
    onClose()
  }

  function finish(result: Result) {
    if (!active.current) return
    onSaved(result)
    close()
  }

  function handleAuth(error: unknown) {
    if (
      error instanceof ApiError &&
      (error.kind === 'auth-required' || error.kind === 'session-ended')
    ) {
      // ApiClient notifies the session provider, which opens UI-03.
      close()
      return true
    }
    return false
  }

  async function check(attempt: ChangeAttempt<Draft>, reason: RecheckReason) {
    setStatus({ kind: 'checking', attempt, reason })
    try {
      const outcome = await recheck(
        attempt,
        reason,
        AbortSignal.timeout(requestTimeoutMs),
      )
      if (!active.current) return
      if (
        reason === 'uncertain' &&
        (outcome.kind === 'applied' || outcome.kind === 'different-result')
      ) {
        setStatus({ kind: 'reconciled', attempt, outcome })
        return
      }
      if (outcome.kind === 'retryable' && reason === 'uncertain') {
        setStatus({ kind: 'retryable', attempt })
        return
      }
      if (
        outcome.kind === 'not-applied' &&
        isStrongETag(outcome.proposal.etag) &&
        (reason === 'uncertain' || outcome.proposal.etag !== attempt.etag)
      ) {
        setStatus({
          kind: 'preview',
          proposal: outcome.proposal,
          notice: reason === 'conflict' ? 'updated' : 'not-applied',
        })
        return
      }
    } catch (error) {
      if (!active.current || handleAuth(error)) return
    }
    setStatus({ kind: 'check-failed', attempt, reason })
  }

  async function send(attempt: ChangeAttempt<Draft>) {
    setStatus({ kind: 'saving', attempt })
    let result: Result
    try {
      result = await submit(attempt, AbortSignal.timeout(requestTimeoutMs))
    } catch (error) {
      if (!active.current || handleAuth(error)) return
      if (error instanceof ApiError) {
        if (
          error.kind === 'version-conflict' &&
          error.problem?.status === 412
        ) {
          await check(attempt, 'conflict')
          return
        }
        if (error.kind === 'validation' && error.problem?.status === 422) {
          onValidation({
            problem: error.problem,
            fields: fieldErrorsByPointer(error.problem),
          })
          close()
          return
        }
        if (
          error.kind === 'missing-version' ||
          (error.problem && error.problem.status < 500)
        ) {
          if (onRejected?.(error)) close()
          else setStatus({ kind: 'rejected', attempt })
          return
        }
      }
      await check(attempt, 'uncertain')
      return
    }
    if (active.current) finish(result)
  }

  function run(action: () => Promise<void>) {
    if (!active.current || busy.current) return
    busy.current = true
    void action()
  }

  function cancel() {
    if (!active.current || busy.current || uncertain) return
    if (status.kind === 'reconciled') finish(status.outcome.result)
    else close()
  }

  function confirm() {
    if (status.kind === 'retryable') {
      const { attempt } = status
      run(() => send(attempt))
      return
    }
    if (
      status.kind === 'preview' &&
      status.proposal.changes.length > 0 &&
      (initialWrite || isStrongETag(status.proposal.etag))
    ) {
      const proposal = status.proposal
      run(() => send({ ...proposal, idempotencyKey: crypto.randomUUID() }))
    }
  }

  function retryCheck() {
    if (status.kind === 'check-failed') {
      const { attempt, reason } = status
      run(() => check(attempt, reason))
    }
  }

  return { status, proposal, working, uncertain, confirm, cancel, retryCheck }
}
