import { Container, Text } from '@gravity-ui/uikit'
import { layoutSizes } from '@/shared/config'
export function LoginPage() {
  return (
    <Container
      as="main"
      maxWidth={layoutSizes.content}
      paddingBlock={{ xs: 'spacing-4', m: 'spacing-6', xl: 'spacing-8' }}
      style={{ boxSizing: 'border-box' }}
    >
      <Text as="h1" variant="header-1">
        Вход
      </Text>
    </Container>
  )
}
