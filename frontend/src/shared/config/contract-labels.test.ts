import { expect, it } from 'vitest'
import { contractLabel, contractLabels } from './contract-labels'

it.each([
  ['taskStatus', 'intake active wait blocked done cancelled duplicate unknown'],
  [
    'eventKind',
    'created field_change comment link_change moved ready reviewer_assigned reviewer_removed review approval head_pushed merged closed reopened pipeline_started pipeline_finished attempt_started attempt_finished version_created',
  ],
  [
    'linkMethod',
    'source_explicit user_confirmed branch_key title_key time_window path_match',
  ],
  ['linkStatus', 'candidate confirmed rejected'],
  ['ciOutcome', 'running success failed canceled skipped unknown'],
  [
    'failureKind',
    'script_failure runner_system_failure stuck_or_timeout_failure job_execution_timeout scheduler_failure api_failure data_integrity_failure unknown_failure',
  ],
  [
    'reviewWait',
    'no_reviewer awaiting_first_response awaiting_re_review responded unknown',
  ],
] as const)('has Russian text for all %s codes', (group, codes) => {
  for (const code of codes.split(' ')) {
    const label = contractLabel(contractLabels[group], code)
    expect(label).toMatch(/[а-я]/iu)
    expect(label).not.toContain('Неизвестное значение')
  }
})

it.each(['future_code', 'constructor', '__proto__'])(
  'includes the raw unknown code %s in the fallback',
  (code) => {
    expect(contractLabel(contractLabels.eventKind, code)).toBe(
      `Неизвестное значение (${code})`,
    )
  },
)
