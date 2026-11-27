import { Button } from '@gravity-ui/uikit'
import type { RefObject } from 'react'
import type { useConnectionProbe } from '../api/use-connection-probe'
import { ProbeProgress, ProbeResult } from './probe-result'

export function ConnectionProbePanel({
  probe,
  blocked,
  saveFirst,
  restoring,
  opener,
  probeButton,
  check,
  open,
}: {
  probe: ReturnType<typeof useConnectionProbe>
  blocked: boolean
  saveFirst: boolean
  restoring: boolean
  opener: RefObject<HTMLButtonElement | null>
  probeButton: RefObject<HTMLButtonElement | null>
  check: () => void
  open: () => void
}) {
  return (
    <fieldset className="cp-connection__section">
      <legend>Проверка подключения</legend>
      {restoring && (
        <p>
          Доступ к источнику потерян. Замените и сохраните учётные данные, затем
          проверьте подключение.
        </p>
      )}
      {saveFirst && <p>Сохраните настройки перед проверкой подключения.</p>}
      {probe.notice && <p role="alert">{probe.notice}</p>}
      {probe.obsolete && probe.data && (
        <p role="status">
          Результат проверки устарел.{' '}
          {probe.versionChanged
            ? 'Загрузите актуальные настройки и повторите проверку.'
            : 'Запустите новую проверку.'}
        </p>
      )}
      {probe.readError ? (
        <div role="alert">
          <p>Не удалось загрузить результат проверки.</p>
          <Button
            type="button"
            onClick={probe.refresh}
            disabled={probe.reading}
          >
            Перечитать результат
          </Button>
        </div>
      ) : probe.inProgress || probe.pending === 'start' ? (
        <ProbeProgress running={probe.data?.status === 'running'} />
      ) : (
        probe.data && !probe.obsolete && <ProbeResult probe={probe.data} />
      )}
      <div className="cp-connection__actions">
        <Button
          type="button"
          size="xl"
          view="outlined"
          ref={probeButton}
          disabled={blocked || probe.inProgress}
          onClick={check}
        >
          Проверить подключение
        </Button>
        <Button
          type="button"
          size="xl"
          view="action"
          ref={opener}
          disabled={blocked || !probe.ready}
          onClick={open}
        >
          Подтвердить и начать сбор
        </Button>
      </div>
    </fieldset>
  )
}
