import { Button } from '@gravity-ui/uikit'
import type { RefObject } from 'react'
import { DialogFrame } from '@/shared/ui'

export function ConnectionDialog({
  kind,
  busy,
  reloadFailed,
  accept,
  close,
  opener,
}: {
  kind: 'draft' | 'conflict'
  busy: boolean
  reloadFailed: boolean
  accept: () => void
  close: () => void
  opener: RefObject<HTMLButtonElement | null>
}) {
  const conflict = kind === 'conflict'
  return (
    <DialogFrame
      title={
        conflict
          ? 'Данные подключения изменились'
          : 'Подтвердите изменение подключения'
      }
      description={
        conflict
          ? 'Загрузите актуальную версию подключения. Она заменит изменения в форме.'
          : 'Подключение вернётся в черновик. Чтобы возобновить сбор, потребуется снова проверить подключение и подтвердить его.'
      }
      focusSafeAction
      openerRef={opener}
      onCancel={() => {
        if (!busy) close()
      }}
      actions={
        <>
          <Button view="outlined" size="xl" onClick={close} disabled={busy}>
            Отмена
          </Button>
          <Button
            view="action"
            size="xl"
            onClick={accept}
            loading={busy}
            disabled={busy}
          >
            {conflict ? 'Загрузить актуальную версию' : 'Подтвердить'}
          </Button>
        </>
      }
    >
      {reloadFailed && (
        <p role="alert">
          Не удалось загрузить актуальную версию. Повторите загрузку.
        </p>
      )}
    </DialogFrame>
  )
}
