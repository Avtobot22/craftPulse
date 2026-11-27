import {
  CircleCheck,
  CircleMinus,
  CircleQuestion,
  CircleXmark,
  Clock,
} from '@gravity-ui/icons'
import { Flex, Text } from '@gravity-ui/uikit'
import type { ComponentProps } from 'react'
import { StatusLabel, StatusTime, TechnicalValue } from '@/shared/ui'
import type { InstallationProtocol } from '../api/installation-protocol'
import './installation-page.css'

type Check = InstallationProtocol['checks'][number]

const outcomeLabels = {
  running: { text: 'Выполняется', icon: Clock, theme: 'info' },
  passed: { text: 'Пройдено', icon: CircleCheck, theme: 'success' },
  failed: { text: 'Не пройдено', icon: CircleXmark, theme: 'danger' },
  skipped: { text: 'Пропущено', icon: CircleMinus, theme: 'normal' },
  unknown: {
    text: 'Неизвестный исход',
    icon: CircleQuestion,
    theme: 'unknown',
  },
} satisfies Record<
  InstallationProtocol['outcome'] | Check['outcome'],
  ComponentProps<typeof StatusLabel>
>

const checkLabels: Record<Check['check_kind'], string> = {
  site: 'Сайт',
  adapter: 'Адаптер',
  adapter_store: 'Хранилище адаптеров',
  identity_provider: 'Провайдер входа',
  update_source: 'Источник обновлений',
  database: 'База данных',
  object_store: 'Объектное хранилище',
  runner_network: 'Сеть раннера',
  unknown: 'Неизвестная проверка',
}

export function ProtocolDetails({
  protocol,
}: {
  protocol: InstallationProtocol
}) {
  return (
    <>
      <TechnicalValue label="SHA выпуска" value={protocol.release_sha} />
      <dl className="cp-installation__summary">
        <div>
          <dt>Начало</dt>
          <dd>
            <StatusTime value={protocol.started_at} />
          </dd>
        </div>
        <div>
          <dt>Окончание</dt>
          <dd>
            <StatusTime value={protocol.finished_at} />
          </dd>
        </div>
        <div>
          <dt>Общий исход</dt>
          <dd>
            <StatusLabel {...outcomeLabels[protocol.outcome]} />
          </dd>
        </div>
      </dl>
      <Flex as="section" direction="column" gap="spacing-2">
        <Text id="installation-checks" as="h2" variant="subheader-2">
          Проверки
        </Text>
        {protocol.checks.length === 0 ? (
          <Text as="p">Результатов проверок пока нет.</Text>
        ) : (
          <table
            className="cp-installation__checks"
            aria-labelledby="installation-checks"
          >
            <thead>
              <tr>
                <th scope="col">Проверка</th>
                <th scope="col">Исход</th>
                <th scope="col">Код причины</th>
              </tr>
            </thead>
            <tbody>
              {protocol.checks.map((check, index) => (
                <tr key={index}>
                  <th scope="row">{checkLabels[check.check_kind]}</th>
                  <td>
                    <StatusLabel {...outcomeLabels[check.outcome]} />
                  </td>
                  <td>
                    {check.reason_code === null ? (
                      'Нет'
                    ) : (
                      <code>{check.reason_code}</code>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Flex>
    </>
  )
}
