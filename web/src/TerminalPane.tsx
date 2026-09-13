import { useEffect, useImperativeHandle, useRef, forwardRef } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'

const ANSI = {
  reset: '\x1b[0m',
  dim: '\x1b[2m',
  bold: '\x1b[1m',
  gray: '\x1b[38;5;244m',
  slate: '\x1b[38;5;250m',
  cyan: '\x1b[38;5;80m',
  blue: '\x1b[38;5;75m',
  green: '\x1b[38;5;78m',
  amber: '\x1b[38;5;215m',
  red: '\x1b[38;5;203m',
  violet: '\x1b[38;5;141m',
} as const

export interface TermHandle {
  writeLine(text: string): void
  writeLog(opts: {
    time: string
    worker: number
    tag: string
    step: string
    statusCode: number
  }): void
  writeBanner(lines: string[]): void
  writeAccount(email: string, ok: boolean, err?: string): void
  clear(): void
}

function colorForStatus(code: number): string {
  if (!code) return ANSI.gray
  if (code >= 500) return ANSI.red
  if (code >= 400) return ANSI.amber
  if (code >= 300) return ANSI.violet
  return ANSI.green
}

const WORKER_COLORS = [ANSI.cyan, ANSI.violet, ANSI.blue, ANSI.amber, ANSI.green]

export const TerminalPane = forwardRef<TermHandle, { className?: string }>(
  function TerminalPane({ className }, ref) {
    const hostRef = useRef<HTMLDivElement | null>(null)
    const termRef = useRef<Terminal | null>(null)
    const fitRef = useRef<FitAddon | null>(null)

    useEffect(() => {
      if (!hostRef.current) return

      const term = new Terminal({
        convertEol: true,
        cursorBlink: true,
        cursorStyle: 'bar',
        disableStdin: true,
        fontFamily: '"JetBrains Mono", ui-monospace, Menlo, Consolas, monospace',
        fontSize: 12.5,
        lineHeight: 1.5,
        scrollback: 5000,
        theme: {
          background: '#0a0d13',
          foreground: '#c9d4e3',
          cursor: '#4da3ff',
          selectionBackground: '#1f3350',
          black: '#0a0d13',
          brightBlack: '#3b4654',
        },
      })

      const fit = new FitAddon()
      term.loadAddon(fit)
      term.open(hostRef.current)
      fit.fit()

      termRef.current = term
      fitRef.current = fit

      const ro = new ResizeObserver(() => {
        try {
          fit.fit()
        } catch {
          // container not measurable yet
        }
      })
      ro.observe(hostRef.current)

      return () => {
        ro.disconnect()
        term.dispose()
        termRef.current = null
        fitRef.current = null
      }
    }, [])

    useImperativeHandle(
      ref,
      (): TermHandle => ({
        writeLine(text) {
          termRef.current?.writeln(text)
        },
        writeLog({ time, worker, tag, step, statusCode }) {
          const t = termRef.current
          if (!t) return
          const wc = WORKER_COLORS[(worker - 1) % WORKER_COLORS.length]
          const sc = colorForStatus(statusCode)
          const code = statusCode ? ` ${sc}${statusCode}${ANSI.reset}` : ''
          t.writeln(
            `${ANSI.gray}${time}${ANSI.reset} ` +
              `${wc}W${worker}${ANSI.reset} ` +
              `${ANSI.dim}${tag.padEnd(7)}${ANSI.reset} ` +
              `${ANSI.slate}${step}${ANSI.reset}${code}`,
          )
        },
        writeAccount(email, ok, err) {
          const t = termRef.current
          if (!t) return
          if (ok) {
            t.writeln(`${ANSI.green}${ANSI.bold}  ✔ ${email}${ANSI.reset}`)
          } else {
            t.writeln(
              `${ANSI.red}${ANSI.bold}  ✘ ${email || '(no email)'}${ANSI.reset}` +
                (err ? ` ${ANSI.dim}${err}${ANSI.reset}` : ''),
            )
          }
        },
        writeBanner(lines) {
          const t = termRef.current
          if (!t) return
          lines.forEach((l) => t.writeln(`${ANSI.cyan}${l}${ANSI.reset}`))
        },
        clear() {
          termRef.current?.clear()
        },
      }),
      [],
    )

    return <div ref={hostRef} className={className} />
  },
)
