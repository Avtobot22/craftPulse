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
    wide = false,
    openerRef,
  } = props
  const { status, proposal, working, uncertain, confirm, cancel, retryCheck } =
    useChangeConfirmation(props)
  const canSubmit = status.kind === 'preview' && isStrongETag(proposal.etag)
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
      description="Проверьте изменения перед сохранением."
      wide={wide}
      focusSafeAction={dangerous}
      onCancel={cancel}
      openerRef={openerRef}
      actions={
        status.kind === 'applied' ? (
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
                {status.kind === 'saving' ? 'Сохранение' : 'Подтвердить'}
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
      {status.kind === 'preview' && !canSubmit && (
        <div role="alert">
          <Alert
            theme="danger"
            title="Версия недоступна. Перечитайте данные в форме."
          />
        </div>
      )}
      {status.kind === 'rejected' && (
        <div role="alert">
          <Alert
            theme="danger"
            title="Изменение отклонено. Вернитесь в форму."
          />
        </div>
      )}
      {status.kind === 'applied' && (
        <div role="status">
          <Alert theme="success" title="Изменение применилось" />
        </div>
      )}
      <dl className="cp-settings-dialog__details">
        <dt>Область</dt>
        <dd>{scope}</dd>
        <dt>Версия для сравнения</dt>
        <dd>{proposal.etag}</dd>
        <dt>Последствия</dt>
        <dd>{consequences}</dd>
      </dl>
      <ul className="cp-settings-dialog__changes" aria-label="Изменения">
        {proposal.changes.map(({ label, before, after }, index) => (
          <li key={index}>
            <strong>{label}</strong>
            <span>Было: {before}</span>
            <span>Станет: {after}</span>
          </li>
        ))}
      </ul>
    </DialogFrame>
  )
}
