import { Alert, Button } from '@gravity-ui/uikit'
import { isStrongETag } from '@/shared/api'
import { DialogFrame } from './dialog-frame'
import { useChangeConfirmation } from './use-change-confirmation'
import type { ChangeConfirmationDialogProps } from './change-confirmation'

/** Mount once per confirmation, with the proposal captured from the form. */
export function ChangeConfirmationDialog<Draft, Result>(
  props: ChangeConfirmationDialogProps<Draft, Result>,
) {
  const {
    title,
    scope,
    consequences,
    dangerous = false,
    initialWrite = false,
    wide = false,
    openerRef,
  } = props
  const { status, proposal, working, uncertain, confirm, cancel, retryCheck } =
    useChangeConfirmation(props)
  const outcome = status.kind === 'reconciled' ? status.outcome : undefined
  const showAttempt = outcome?.kind !== 'different-result'
  const unchanged = status.kind === 'preview' && proposal.changes.length === 0
  const missingVersion = !initialWrite && !isStrongETag(proposal.etag)
  const retryable = status.kind === 'retryable'
  const canSubmit =
    retryable || (status.kind === 'preview' && !missingVersion && !unchanged)
  let confirmLabel = 'Подтвердить'
  if (status.kind === 'saving') confirmLabel = 'Сохранение'
  if (retryable) confirmLabel = 'Повторить сохранение'
  let progressMessage = ''
  if (status.kind === 'saving') progressMessage = 'Сохранение'
  if (status.kind === 'checking') {
    progressMessage =
      status.reason === 'conflict'
        ? 'Данные изменились. Загружаем актуальную версию.'
        : 'Исход сохранения неизвестен. Проверяем актуальную версию.'
  }

  return (
    <DialogFrame
      title={title}
      description={
        outcome
          ? 'Результат проверки сохранения.'
          : 'Проверьте изменения перед сохранением.'
      }
      wide={wide}
      focusSafeAction={dangerous}
      onCancel={cancel}
      openerRef={openerRef}
      actions={
        outcome ? (
          <Button view="action" size="xl" onClick={cancel}>
            Готово
          </Button>
        ) : (
          <>
            <Button
              view="outlined"
              size="xl"
              disabled={working || uncertain}
              onClick={cancel}
            >
              Отмена
            </Button>
            {status.kind === 'check-failed' ? (
              <Button view="action" size="xl" onClick={retryCheck}>
                Перечитать ресурс
              </Button>
            ) : (
              <Button
                view="action"
                size="xl"
                disabled={!canSubmit}
                onClick={confirm}
              >
                {confirmLabel}
              </Button>
            )}
          </>
        )
      }
    >
      <p role="status" className="cp-settings-dialog__progress">
        {progressMessage}
      </p>
      {status.kind === 'check-failed' && (
        <div role="alert">
          <Alert
            theme="warning"
            title={
              uncertain
                ? 'Исход сохранения пока неизвестен'
                : 'Не удалось загрузить актуальную версию'
            }
            message="Перечитайте ресурс перед повторным подтверждением."
          />
        </div>
      )}
      {retryable && (
        <div role="alert">
          <Alert
            theme="warning"
            title="Исход сохранения неизвестен"
            message="Повторите тот же запрос или вернитесь к черновику."
          />
        </div>
      )}
      {status.kind === 'preview' && status.notice && (
        <div role="alert">
          <Alert
            theme="warning"
            title={
              status.notice === 'updated'
                ? 'Данные изменились. Просмотрите актуальную версию'
                : 'Изменение не применилось'
            }
            message="Проверьте сравнение перед новым подтверждением."
          />
        </div>
      )}
      {status.kind === 'preview' && missingVersion && (
        <div role="alert">
          <Alert
            theme="danger"
            title="Версия недоступна. Перечитайте данные в форме."
          />
        </div>
      )}
      {unchanged && <p role="status">Изменений нет</p>}
      {status.kind === 'rejected' && (
        <div role="alert">
          <Alert
            theme="danger"
            title="Изменение отклонено. Вернитесь в форму."
          />
        </div>
      )}
      {outcome && (
        <div role="status">
          {outcome.kind === 'applied' ? (
            <Alert theme="success" title="Изменение применилось" />
          ) : (
            <Alert
              theme="warning"
              title={outcome.title}
              message={outcome.message}
            />
          )}
        </div>
      )}
      <dl className="cp-settings-dialog__details">
        <dt>Область</dt>
        <dd>{scope}</dd>
        {showAttempt && !initialWrite && (
          <>
            <dt>Версия для сравнения</dt>
            <dd>{proposal.etag}</dd>
          </>
        )}
        {showAttempt && proposal.baseline && (
          <>
            <dt>{proposal.baseline.label}</dt>
            <dd className="cp-settings-dialog__baseline">
              {proposal.baseline.value}
            </dd>
          </>
        )}
        {showAttempt && (
          <>
            <dt>Последствия</dt>
            <dd>{consequences}</dd>
          </>
        )}
      </dl>
      {showAttempt && (
        <ul className="cp-settings-dialog__changes" aria-label="Изменения">
          {[...(proposal.referenceChanges ?? []), ...proposal.changes].map(
            ({ label, before, after }, index) => (
              <li key={index}>
                <strong>{label}</strong>
                <span>Было: {before}</span>
                <span>Станет: {after}</span>
              </li>
            ),
          )}
        </ul>
      )}
    </DialogFrame>
  )
}
