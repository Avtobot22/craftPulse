import type { GridProps, LayoutTheme } from '@gravity-ui/uikit'

export const layoutSizes = {
  content: '1600px',
  navigation: '240px',
  form: '720px',
  reading: '80ch',
} as const

export const layoutTheme = {
  breakpoints: { m: 768, xl: 1280 },
  components: {
    container: {
      gutters: 'spacing-4',
      media: { m: { gutters: 'spacing-6' }, xl: { gutters: 'spacing-8' } },
    },
  },
} satisfies LayoutTheme

export const contentGridProps = {
  columns: {
    xs: 'repeat(4, minmax(0, 1fr))',
    m: 'repeat(8, minmax(0, 1fr))',
    xl: 'repeat(12, minmax(0, 1fr))',
  },
  gap: { xs: 'spacing-4', xl: 'spacing-6' },
} satisfies Pick<GridProps, 'columns' | 'gap'>
