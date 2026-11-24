import type { DefaultPropsMap } from '@gravity-ui/uikit'

export const uikitDefaults = {
  Button: { size: 'xl', style: { fontWeight: 600 } },
  ClipboardButton: { size: 'xl', style: { fontWeight: 600 } },
  Checkbox: { size: 'l', style: { minHeight: 44, minWidth: 44 } },
  Radio: { size: 'l', style: { minHeight: 44, minWidth: 44 } },
  TextInput: { size: 'xl' },
  TextArea: { size: 'xl', minRows: 5 },
  Select: { size: 'xl', popupClassName: 'cp-select-popup' },
  Tooltip: {
    openDelay: 300,
    offset: 8,
    style: {
      maxWidth: 'min(320px, calc(100vw - 32px))',
      fontSize: '0.875rem',
      lineHeight: '1.25rem',
    },
  },
  Link: {
    underline: true,
    visitable: true,
    style: {
      display: 'inline-flex',
      alignItems: 'center',
      minHeight: 44,
      outlineOffset: 2,
    },
  },
} satisfies DefaultPropsMap
