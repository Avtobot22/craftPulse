import { Flex, Spin, Text } from '@gravity-ui/uikit'
import { Link } from 'react-router'
import { CapabilityList } from '@/entities/connection'
import { adminLabels, contractLabel, routePaths } from '@/shared/config'
import { ContractReason } from '@/shared/ui'
import {
  capabilityGroups,
  groupedCapabilities,
  probeRemedies,
} from '../model/probe-result'
import type { ConnectionProbe } from '../model/probe-result'

export function ProbeProgress({ running }: { running: boolean }) {
  return (
    <Flex role="status" alignItems="center" gap="spacing-2">
      <Spin size="m" />
      <span>
        {running ? 'Пробное чтение источника' : 'Ожидание пробного чтения'}
      </span>
    </Flex>
  )
}

export function ProbeResult({ probe }: { probe: ConnectionProbe }) {
  if (probe.status === 'failed') {
    const reason = probe.reason_code
    return (
      <div role="alert">
        <p>Проверка не пройдена</p>
        {reason === 'edition_mismatch' ? (
          <p>Это другая редакция, этот адаптер её не поддерживает</p>
        ) : reason ? (
          <ContractReason code={reason} labels={adminLabels.probeReason} />
        ) : (
          <p>Причина не указана</p>
        )}
        <p>
          {reason && Object.hasOwn(probeRemedies, reason)
            ? contractLabel(probeRemedies, reason)
            : 'Проверьте настройки подключения или повторите проверку позже.'}
        </p>
        {reason === 'edition_mismatch' && (
          <Link to={routePaths.importPackage}>Универсальный импорт</Link>
        )}
      </div>
    )
  }
  if (probe.status !== 'succeeded') return null

  const extra = probe.capabilities.filter(
    ({ capability }) => !groupedCapabilities.has(capability),
  )
  const checked = new Set(
    probe.capabilities.map(({ capability }) => capability),
  )
  return (
    <>
      <p role="status">Проверка завершена</p>
      <dl className="cp-connection__details">
        <dt>Редакция</dt>
        <dd>{probe.edition ?? 'Нет данных'}</dd>
        <dt>Версия источника</dt>
        <dd>{probe.source_version ?? 'Нет данных'}</dd>
        <dt>Версия контракта</dt>
        <dd>{probe.contract_version ?? 'Нет данных'}</dd>
      </dl>
      {probe.capabilities.some(
        ({ state }) => state === 'partial' || state === 'missing',
      ) && (
        <p>Показатели, которым нужны недостающие данные, будут неизвестны.</p>
      )}
      <div className="cp-connection__capabilities">
        {capabilityGroups.map(({ title, names }) => (
          <section key={title}>
            <Text as="h3" variant="subheader-2">
              {title}
            </Text>
            <CapabilityList
              capabilities={probe.capabilities.filter(({ capability }) =>
                names.some((name) => name === capability),
              )}
              unchecked={names.filter((name) => !checked.has(name))}
            />
          </section>
        ))}
        {extra.length > 0 && (
          <section>
            <Text as="h3" variant="subheader-2">
              Другие возможности
            </Text>
            <CapabilityList capabilities={extra} />
          </section>
        )}
      </div>
    </>
  )
}
