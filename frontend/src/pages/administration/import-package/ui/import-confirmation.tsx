import { Button, Spin } from '@gravity-ui/uikit'
import { useRef } from 'react'
import type { RefObject } from 'react'
import { DialogFrame } from '@/shared/ui'

export function ImportConfirmation({
  project,
  schema,
  file,
  snapshot,
  sending,
  openerRef,
  onConfirm,
  onClose,
}: {
  project: string
  schema: string
  file: File
  snapshot: boolean
  sending: boolean
  openerRef: RefObject<HTMLButtonElement | null>
  onConfirm: () => void
  onClose: () => void
}) {
  const submitted = useRef(false)
  return (
    <DialogFrame
      title="Передать пакет?"
      description="Проверьте параметры и файл перед отправкой."
      openerRef={openerRef}
      returnFocus={(opener) => (submitted.current ? false : opener)}
      onCancel={() => {
        if (!sending) onClose()
      }}
      actions={
        <>
          <Button
            size="xl"
            view="outlined"
            onClick={onClose}
            disabled={sending}
          >
            Вернуться к форме
          </Button>
          <Button
            size="xl"
            view="action"
            onClick={() => {
              submitted.current = true
              onConfirm()
            }}
            disabled={sending}
          >
            Подтвердить отправку
          </Button>
        </>
      }
    >
      <dl className="cp-settings-dialog__details">
        <dt>Проект</dt>
        <dd>{project}</dd>
        <dt>Схема</dt>
        <dd>{schema}</dd>
        <dt>Файл</dt>
        <dd className="cp-import__filename">{file.name}</dd>
        <dt>Охват</dt>
        <dd>
          {snapshot ? 'Только текущее состояние' : 'По содержимому файла'}
        </dd>
      </dl>
      <div className="cp-import__progress" role="status">
        {sending && (
          <>
            <Spin size="s" aria-hidden="true" />
            Передаём файл. Ожидаем квитанцию.
          </>
        )}
      </div>
    </DialogFrame>
  )
}
