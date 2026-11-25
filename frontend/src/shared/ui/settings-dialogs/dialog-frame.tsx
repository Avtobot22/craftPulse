import { useId, useLayoutEffect, useRef } from 'react'
import type { KeyboardEvent, ReactNode, RefObject } from 'react'
import './settings-dialogs.css'

interface DialogFrameProps {
  title: string
  description: string
  children?: ReactNode
  actions: ReactNode
  wide?: boolean
  focusSafeAction?: boolean
  onCancel: () => void
  openerRef?: RefObject<HTMLElement | null> | undefined
  // Return false after a confirmed route change, where the destination owns focus.
  returnFocus?: (opener: HTMLElement | null) => HTMLElement | null | false
}

function focusPageHeading() {
  const heading =
    document.querySelector<HTMLElement>('main h1') ??
    document.querySelector<HTMLElement>('h1')
  if (heading) {
    heading.tabIndex = -1
    heading.focus()
  }
}

function tabStops(dialog: HTMLDialogElement) {
  return [
    ...dialog.querySelectorAll<HTMLElement>('button:not(:disabled), a[href]'),
  ].filter((element) => element.tabIndex >= 0 && !element.closest('[hidden]'))
}

export function DialogFrame({
  title,
  description,
  children,
  actions,
  wide = false,
  focusSafeAction = false,
  onCancel,
  openerRef,
  returnFocus,
}: DialogFrameProps) {
  const id = useId()
  const dialogRef = useRef<HTMLDialogElement>(null)
  const titleRef = useRef<HTMLHeadingElement>(null)
  const focusOptions = useRef({ focusSafeAction, openerRef, returnFocus })

  useLayoutEffect(() => {
    const dialog = dialogRef.current
    if (!dialog) return
    const { focusSafeAction, openerRef, returnFocus } = focusOptions.current
    const opener =
      openerRef?.current ??
      (document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null)
    dialog.showModal()
    const initial = focusSafeAction ? tabStops(dialog)[0] : titleRef.current
    initial?.focus()

    return () => {
      dialog.close()
      const target = returnFocus ? returnFocus(opener) : opener
      if (target === false) return
      const focusAfterClose = document.activeElement
      // The same commit can disable the opener or focus a new form error summary.
      queueMicrotask(() => {
        const focused = document.activeElement
        if (
          dialog.open ||
          (focused !== focusAfterClose && focused !== document.body)
        )
          return
        if (target?.isConnected && !target.matches(':disabled')) target.focus()
        if (
          !target ||
          target === document.body ||
          document.activeElement !== target ||
          target.matches(':disabled')
        ) {
          focusPageHeading()
        }
      })
    }
  }, [])

  // Disabled or replaced actions may lose focus while a request changes state.
  useLayoutEffect(() => {
    const focused = document.activeElement
    if (
      !dialogRef.current?.contains(focused) ||
      focused?.matches(':disabled')
    ) {
      titleRef.current?.focus()
    }
  })

  function keepFocusInside(event: KeyboardEvent<HTMLDialogElement>) {
    if (event.key !== 'Tab') return
    const stops = tabStops(event.currentTarget)
    const index = stops.findIndex(
      (element) => element === document.activeElement,
    )
    if (
      index === -1 ||
      (event.shiftKey ? index === 0 : index === stops.length - 1)
    ) {
      event.preventDefault()
      const target =
        (event.shiftKey ? stops.at(-1) : stops[0]) ?? titleRef.current
      target?.focus()
    }
  }

  return (
    <dialog
      ref={dialogRef}
      className={
        wide
          ? 'cp-settings-dialog cp-settings-dialog_wide'
          : 'cp-settings-dialog'
      }
      aria-labelledby={id + '-title'}
      aria-describedby={id + '-description'}
      onCancel={(event) => {
        event.preventDefault()
        onCancel()
      }}
      onKeyDown={keepFocusInside}
    >
      <h2 ref={titleRef} id={id + '-title'} tabIndex={-1}>
        {title}
      </h2>
      <p id={id + '-description'}>{description}</p>
      {children}
      <footer className="cp-settings-dialog__actions">{actions}</footer>
    </dialog>
  )
}
