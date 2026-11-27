import { Button } from '@gravity-ui/uikit'
import type { RefObject } from 'react'
import { contractLabel } from '@/shared/config'
import { DialogFrame, StatusTime } from '@/shared/ui'
import { scopeLabels } from '../model/connection-draft'
import type { Connection } from '../model/connection-draft'
import { capabilityGroups, capabilityNames } from '../model/probe-result'
import type { ConnectionProbe } from '../model/probe-result'

const availability = [
  { state: 'verified', label: 'Рабочие возможности' },
  { state: 'partial', label: 'Частичные возможности' },
  { state: 'missing', label: 'Недоступные возможности' },
] as const

export function ActivationDialog({
  connection,
  probe,
  busy,
  error,
  opener,
  close,
  confirm,
}: {
  connection: Connection
  probe: ConnectionProbe
  busy: boolean
  error: string | undefined
  opener: RefObject<HTMLButtonElement | null>
  close: () => void
  confirm: () => void
}) {
  const restoring = connection.status === 'access_lost'
  const partial = probe.capabilities.some(
    ({ state }) => state === 'partial' || state === 'missing',
  )
  const unchecked = new Set([
    ...capabilityGroups.flatMap(({ names }) => names),
    ...probe.capabilities.map(({ capability }) => capability),
  ])
  for (const { capability, state } of probe.capabilities) {
    if (availability.some((item) => item.state === state))
      unchecked.delete(capability)
  }

  return (
    <DialogFrame
      title="Подтвердите подключение"
      description={
        restoring
          ? 'Сбор продолжится с сохранённого места.'
          : 'После подтверждения начнётся загрузка истории.'
      }
      focusSafeAction
      openerRef={opener}
      onCancel={() => {
        if (!busy) close()
      }}
      actions={
        <>
          <Button view="outlined" size="xl" disabled={busy} onClick={close}>
            Отмена
          </Button>
          <Button
            view="action"
            size="xl"
            disabled={busy}
            loading={busy}
            onClick={confirm}
          >
            Подтвердить
          </Button>
        </>
      }
    >
      {error && <p role="alert">{error}</p>}
      {partial && <p>Подключение будет частичным.</p>}
      <dl className="cp-settings-dialog__details">
        <dt>Область и период</dt>
        <dd>
          <ul>
            {connection.scopes.map((scope) => (
              <li key={scope.id}>
                {contractLabel(scopeLabels, scope.kind)}{' '}
                {scope.display_name || scope.native_scope_id}
                {' · '}
                {scope.history_from ? (
                  <>
                    С <StatusTime value={scope.history_from} />
                  </>
                ) : (
                  'Вся доступная история'
                )}
              </li>
            ))}
          </ul>
        </dd>
        {availability.map(({ state, label }) => (
          <div key={state}>
            <dt>{label}</dt>
            <dd>
              {capabilityNames(
                probe.capabilities
                  .filter((item) => item.state === state)
                  .map(({ capability }) => capability),
              )}
            </dd>
          </div>
        ))}
        <dt>Не проверено</dt>
        <dd>{capabilityNames([...unchecked])}</dd>
      </dl>
    </DialogFrame>
  )
}
