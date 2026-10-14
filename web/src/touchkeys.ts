// Accessory keys for touch devices.
//
// A soft keyboard has no Ctrl, no Esc and no arrow keys, which makes a shell
// unusable on a phone. These buttons inject the sequences a physical keyboard
// would have produced. Ctrl and Alt are sticky: tap Ctrl, then `c`, and the
// terminal receives \x03.

const KEYS: Array<{ label: string; seq?: string; sticky?: 'ctrl' | 'alt' }> = [
  { label: 'Esc', seq: '\x1b' },
  { label: 'Tab', seq: '\t' },
  { label: 'Ctrl', sticky: 'ctrl' },
  { label: 'Alt', sticky: 'alt' },
  { label: '↑', seq: '\x1b[A' },
  { label: '↓', seq: '\x1b[B' },
  { label: '→', seq: '\x1b[C' },
  { label: '←', seq: '\x1b[D' },
  { label: 'C-c', seq: '\x03' },
  { label: 'C-d', seq: '\x04' },
  { label: '|', seq: '|' },
  { label: '-', seq: '-' },
  { label: '/', seq: '/' },
  { label: '~', seq: '~' },
]

/** Control characters for letters, so sticky Ctrl can compose with the keyboard. */
function control(letter: string): string {
  return String.fromCharCode(letter.charCodeAt(0) & 0x1f)
}

export function mountTouchKeys(root: HTMLElement, send: (data: string) => void): void {
  // Only worth the pixels where there is no hardware keyboard.
  if (!matchMedia('(pointer: coarse)').matches) return

  const bar = document.createElement('div')
  bar.className = 'tf-keys'
  let sticky: 'ctrl' | 'alt' | null = null

  const buttons = new Map<HTMLButtonElement, string>()
  for (const spec of KEYS) {
    const btn = document.createElement('button')
    btn.className = 'tf-key'
    btn.type = 'button'
    btn.textContent = spec.label
    // Keep focus on the terminal: a blurred xterm stops receiving keystrokes.
    btn.addEventListener('pointerdown', (ev) => ev.preventDefault())
    btn.addEventListener('click', () => {
      if (spec.sticky) {
        sticky = sticky === spec.sticky ? null : spec.sticky
        for (const [b, s] of buttons) b.classList.toggle('on', s === spec.label)
        return
      }
      let seq = spec.seq ?? ''
      if (sticky === 'ctrl') seq = control(seq.toLowerCase())
      else if (sticky === 'alt') seq = '\x1b' + seq
      sticky = null
      for (const b of buttons.keys()) b.classList.remove('on')
      send(seq)
    })
    buttons.set(btn, spec.label)
    bar.append(btn)
  }

  // While a sticky modifier is armed, the next keystroke from the soft keyboard
  // gets it too — that is how Ctrl+C is typed on a phone.
  document.addEventListener('keydown', (ev) => {
    if (!sticky) return
    if (ev.key.length !== 1) return
    ev.preventDefault()
    ev.stopPropagation()
    const seq = sticky === 'ctrl' ? control(ev.key.toLowerCase()) : '\x1b' + ev.key
    sticky = null
    for (const b of buttons.keys()) b.classList.remove('on')
    send(seq)
  }, { capture: true })

  root.append(bar)
}