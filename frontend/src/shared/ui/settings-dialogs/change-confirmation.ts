import type { RefObject } from 'react'
import type { ApiError, FieldError, SafeProblem } from '@/shared/api'

export interface ChangePreview {
  label: string
  before: string
  after: string
}

export interface ChangeProposal<Draft> {
  /** Strong ETag of the version shown as the baseline. */
  etag: string
  baseline?: { label: string; value: string }
  /** Display values only. Mask secrets before building the preview. */
  changes: readonly ChangePreview[]
  /** Changes already present in the server version after a refresh. */
  referenceChanges?: readonly ChangePreview[]
  /** Immutable payload represented by the preview. */
  draft: Draft
}

export interface ChangeAttempt<Draft> extends ChangeProposal<Draft> {
  /** Reuse only for the same confirmed request when the endpoint supports it. */
  idempotencyKey: string
}

export type RecheckReason = 'conflict' | 'uncertain'

export type ChangeRecheck<Draft, Result> =
  | { kind: 'not-applied'; proposal: ChangeProposal<Draft> }
  | { kind: 'retryable' }
  | { kind: 'applied'; result: Result }
  | { kind: 'unknown' }

export interface ChangeValidation {
  problem: SafeProblem
  fields: ReadonlyMap<string, readonly FieldError[]>
}

export interface ChangeConfirmationDialogProps<Draft, Result> {
  title: string
  scope: string
  consequences: string
  proposal: ChangeProposal<Draft>
  initialWrite?: boolean
  onRejected?: (error: ApiError) => boolean
  dangerous?: boolean
  wide?: boolean
  openerRef?: RefObject<HTMLElement | null>
  /** Pass the signal to the write request so a timeout can trigger reconciliation. */
  submit: (
    attempt: ChangeAttempt<Draft>,
    signal: AbortSignal,
  ) => Promise<Result>
  /** Read the resource afresh and report an outcome supported by the resource or
   * an operation receipt. After a timeout, an unchanged ETag alone leaves the
   * outcome unknown. Report `not-applied` with that ETag only if the operation
   * status confirms that the write did not apply.
   * For an unchanged strong ETag, `retryable` allows cancel or an explicit
   * replay of the same attempt when the endpoint guarantees idempotency.
   * After a conflict, the new preview requires a different strong ETag.
   */
  recheck: (
    attempt: ChangeAttempt<Draft>,
    reason: RecheckReason,
    signal: AbortSignal,
  ) => Promise<ChangeRecheck<Draft, Result>>
  onSaved: (result: Result) => void
  /** The form renders the summary and links each JSON Pointer to its field. */
  onValidation: (validation: ChangeValidation) => void
  onClose: () => void
}
