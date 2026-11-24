import { Card, ClipboardButton, Flex, Text } from '@gravity-ui/uikit'
import { useState } from 'react'

export type TechnicalValueProps = { label: string } & (
  { value: string; hidden?: false } | { hidden: true; value?: never }
)
export function TechnicalValue(props: TechnicalValueProps) {
  const [result, setResult] = useState<{ value: string; ok: boolean } | null>(
    null,
  )
  const current = !props.hidden && result?.value === props.value ? result : null
  return (
    <Card className="cp-technical-value">
      <Flex
        direction="column"
        alignItems="flex-start"
        gap="spacing-2"
        padding="spacing-4"
      >
        <Text as="div" variant="caption-1">
          {props.label}
        </Text>
        <Text as="code" variant="code-1" className="cp-technical-value__text">
          {props.hidden ? 'Значение скрыто' : props.value}
        </Text>
        {!props.hidden && (
          <ClipboardButton
            text={props.value}
            size="xl"
            view="outlined"
            tooltipInitialText={`Скопировать: ${props.label}`}
            tooltipSuccessText="Скопировано"
            onCopy={(value, ok) => {
              setResult({ value, ok })
            }}
          >
            Копировать
          </ClipboardButton>
        )}
        <span role={current?.ok === false ? 'alert' : 'status'}>
          {current
            ? current.ok
              ? 'Скопировано'
              : 'Не удалось скопировать. Выделите и скопируйте значение.'
            : ''}
        </span>
      </Flex>
    </Card>
  )
}
