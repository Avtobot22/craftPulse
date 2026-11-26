import {
  CircleCheck,
  CircleXmark,
  Clock,
  FileMagnifier,
  FolderOpen,
  PlugConnection,
  CircleMinus,
} from '@gravity-ui/icons'
import {
  Alert,
  Box,
  Flex,
  Icon,
  PlaceholderContainer,
  Text,
} from '@gravity-ui/uikit'
import type { ReactNode } from 'react'
import { layoutSizes } from '@/shared/config'

const emptyReasons = {
  filters: { title: 'Нет объектов по фильтрам', icon: FileMagnifier },
  source: { title: 'Источник не настроен', icon: PlugConnection },
  history: { title: 'История ещё не получена', icon: FolderOpen },
  inapplicable: { title: 'Результат неприменим', icon: CircleMinus },
} as const
export interface EmptyStateProps {
  reason: keyof typeof emptyReasons
  title?: string
  description?: string
  action?: { label: string; onClick: () => void }
}
export function EmptyState({
  reason,
  title,
  description,
  action,
}: EmptyStateProps) {
  const { title: defaultTitle, icon } = emptyReasons[reason]
  const heading = title ?? defaultTitle
  return (
    <Box as="section" role="status" aria-label={heading}>
      <PlaceholderContainer
        className="cp-empty-state"
        size="s"
        align="left"
        direction="column"
        title={heading}
        {...(description
          ? {
              description: (
                <Text variant="caption-1" style={{ overflowWrap: 'anywhere' }}>
                  {description}
                </Text>
              ),
            }
          : {})}
        image={<Icon data={icon} size={24} />}
        actions={
          action
            ? [
                {
                  text: action.label,
                  view: 'outlined',
                  onClick: action.onClick,
                },
              ]
            : undefined
        }
      />
    </Box>
  )
}
export interface ErrorStateProps {
  /** A caller-authored safe message. Never pass Problem.detail or response bodies. */
  reason: string
  correlationId?: string
  onRetry: () => void
  retrying?: boolean
  lastSuccess?: string
}
export function ErrorState({
  reason,
  correlationId,
  onRetry,
  retrying = false,
  lastSuccess,
}: ErrorStateProps) {
  return (
    <Box as="section" role="alert" aria-label="Ошибка">
      <Alert
        className="cp-error-state"
        theme="danger"
        title="Не удалось выполнить операцию"
        icon={
          <Icon
            data={CircleXmark}
            size={24}
            color="danger-heavy"
            style={{ verticalAlign: 'middle' }}
          />
        }
        message={
          <Flex
            direction="column"
            gap="spacing-2"
            maxWidth={layoutSizes.reading}
          >
            <Text as="p" variant="caption-1" style={{ margin: 0 }}>
              {reason}
            </Text>
            {lastSuccess && (
              <Text as="p" variant="caption-1" style={{ margin: 0 }}>
                Последняя удачная операция: {lastSuccess}
              </Text>
            )}
            {correlationId && (
              <Text
                as="p"
                variant="code-1"
                className="cp-technical-value__text"
                style={{ margin: 0 }}
              >
                Код диагностики (correlation_id): {correlationId}
              </Text>
            )}
          </Flex>
        }
        actions={
          <Alert.Actions>
            <Alert.Action
              size="xl"
              view="outlined"
              disabled={retrying}
              onClick={onRetry}
            >
              {retrying ? 'Повтор выполняется' : 'Повторить'}
            </Alert.Action>
          </Alert.Actions>
        }
      />
    </Box>
  )
}
export interface StaleStateProps {
  snapshotAt: string
  children?: ReactNode
  currentError?: ReactNode
}
export function StaleState({
  snapshotAt,
  children,
  currentError,
}: StaleStateProps) {
  return (
    <Flex as="section" direction="column" gap="spacing-4">
      <div role="status" aria-label="Устаревшие данные">
        <Alert
          theme="warning"
          title="Устаревшие данные"
          icon={
            <Icon data={Clock} size={20} style={{ verticalAlign: 'middle' }} />
          }
          message={
            <>
              Последний снимок: <time dateTime={snapshotAt}>{snapshotAt}</time>
            </>
          }
        />
      </div>
      {children}
      {currentError}
    </Flex>
  )
}
export type SuccessStateProps = { message: string } & (
  { version: string; date?: string } | { date: string; version?: string }
)
export function SuccessState({ message, version, date }: SuccessStateProps) {
  return (
    <div role="status" aria-label="Успех">
      <Alert
        theme="success"
        title={message}
        icon={
          <Icon
            data={CircleCheck}
            size={20}
            color="positive-heavy"
            style={{ verticalAlign: 'middle' }}
          />
        }
        message={
          <>
            {version && (
              <Text as="p" variant="code-1">
                Версия: {version}
              </Text>
            )}
            {date && (
              <Text as="p" variant="caption-1">
                Дата: <time dateTime={date}>{date}</time>
              </Text>
            )}
          </>
        }
      />
    </div>
  )
}
