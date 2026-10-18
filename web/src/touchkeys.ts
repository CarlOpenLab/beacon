// Accessory keys for touch devices — and, since the terminal itself is a
// display surface, for every device: a text box cannot produce Esc, Tab,
// Ctrl or the arrows, so these buttons inject the sequences a physical
// keyboard would have produced. Ctrl and Alt are sticky: tap Ctrl, then `c`,
// and the terminal receives \x03.
//
// The bar is one flat row of equal keys, and 'sep' is the hairline that says
// where one kind of key stops and the next begins — the separator carries the
// grouping so the keys themselves do not have to be tinted by position.

type KeySpec = { label: string; seq?: string; sticky?: 'ctrl' | 'alt' }

const KEYS: Array<KeySpec | 'sep'> = [
  { label: 'Esc', seq: '\x1b' },
  { label: 'Tab', seq: '\t' },
  'sep',
  { label: 'Ctrl', sticky: 'ctrl' },
  { label: 'Alt', sticky: 'alt' },
  'sep',
  { label: '↑', seq: '\x1b[A' },
  { label: '↓', seq: '\x1b[B' },
  { label: '→', seq: '\x1b[C' },
  { label: '←', seq: '\x1b[D' },
  'sep',
  { label: 'C-c', seq: '\x03' },
  { label: 'C-d', seq: '\x04' },
  'sep',
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
  const bar = document.createElement('div')
  bar.className = 'tf-keys'
  let sticky: 'ctrl' | 'alt' | null = null

  const buttons = new Map<HTMLButtonElement, string>()
  for (const spec of KEYS) {
    if (spec === 'sep') {
      const sep = document.createElement('span')
      sep.className = 'tf-ksep'
      bar.append(sep)
      continue
    }
    const btn = document.createElement('button')
    btn.className = 'tf-key'
    btn.type = 'button'
    btn.textContent = spec.label
    // Keep focus in the composer: a button click would otherwise blur the
    // text box the user is typing commands into.
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