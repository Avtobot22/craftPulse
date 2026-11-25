import type { RefObject } from 'react'
import type { FieldError, SafeProblem } from '@/shared/api'

export interface ChangePreview {
  label: string
  before: string
  after: string
}

export interface ChangeProposal<Draft> {
  /** Strong ETag of the version shown as the baseline. */
  etag: string
  /** Display values only. Mask secrets before building the preview. */
  changes: readonly ChangePreview[]
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
  dangerous?: boolean
  wide?: boolean
  openerRef?: RefObject<HTMLElement | null>
  /** Pass the signal to the write request so a timeout can trigger reconciliation. */
  submit: (
    attempt: ChangeAttempt<Draft>,
    signal: AbortSignal,
  ) => Promise<Result>
  /** Read the resource afresh. Compare it with the draft and report only an outcome
   * supported by the resource or an operation receipt. A new preview requires a
   * different strong ETag. Reading the same ETag after a timeout leaves the outcome unknown.
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
