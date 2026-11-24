import { Box, Flex, Skeleton, Spin, Text } from '@gravity-ui/uikit'
import type { CSSProperties } from 'react'

export interface LoadingStateProps {
  /** Match the loaded block's geometry. */
  blocks?: readonly {
    width: NonNullable<CSSProperties['width']>
    height: NonNullable<CSSProperties['height']>
  }[]
  minHeight?: CSSProperties['minHeight']
}
export function LoadingState({
  blocks = [{ width: '100%', height: 160 }],
  minHeight = 160,
}: LoadingStateProps) {
  return (
    <Box
      as="section"
      aria-label="Загрузка"
      style={{ minHeight, position: 'relative' }}
    >
      <Flex
        role="status"
        alignItems="center"
        justifyContent="center"
        gap="spacing-2"
        style={{ position: 'absolute', inset: 0, pointerEvents: 'none' }}
      >
        <Spin size="s" style={{ width: 20, height: 20 }} />
        <Text variant="caption-1">Загрузка</Text>
      </Flex>
      <Flex wrap gap="spacing-4" aria-hidden="true">
        {blocks.map((block, i) => (
          <Skeleton key={i} width={block.width} height={block.height} />
        ))}
      </Flex>
    </Box>
  )
}
