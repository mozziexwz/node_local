import {ReactNode, useEffect, useState} from 'react'

export function Card(props: {title?: ReactNode, actions?: ReactNode, children: ReactNode, className?: string}) {
    return (
        <section className={'card ' + (props.className ?? '')}>
            {(props.title || props.actions) && (
                <header className="card-head">
                    <h2>{props.title}</h2>
                    <div className="card-actions">{props.actions}</div>
                </header>
            )}
            {props.children}
        </section>
    )
}

export function Banner(props: {
    kind: 'error' | 'warn' | 'info'
    children: ReactNode
    onClose?: () => void
}) {
    return (
        <div className={'banner ' + props.kind} role={props.kind === 'error' ? 'alert' : 'status'}>
            <div className="banner-text">{props.children}</div>
            {props.onClose && <button className="link" onClick={props.onClose}>知道了</button>}
        </div>
    )
}

export function Switch(props: {checked: boolean, disabled?: boolean, onChange: (v: boolean) => void, label: string}) {
    return (
        <label className={'switch' + (props.disabled ? ' disabled' : '')}>
            <input type="checkbox" checked={props.checked} disabled={props.disabled}
                   onChange={e => props.onChange(e.target.checked)} aria-label={props.label}/>
            <span className="track"><span className="thumb"/></span>
        </label>
    )
}

export interface ConfirmRequest {
    title: string
    message: ReactNode
    confirm: string
    danger?: boolean
    resolve: (ok: boolean) => void
}

// Confirm renders the pending confirmation, if any, as a modal dialog.
export function Confirm(props: {request: ConfirmRequest | null, onDone: () => void}) {
    const r = props.request
    useEffect(() => {
        if (!r) return
        const onKey = (e: KeyboardEvent) => {
            if (e.key === 'Escape') {
                r.resolve(false)
                props.onDone()
            }
        }
        window.addEventListener('keydown', onKey)
        return () => window.removeEventListener('keydown', onKey)
    }, [r, props])
    if (!r) return null
    const close = (ok: boolean) => {
        r.resolve(ok)
        props.onDone()
    }
    return (
        <div className="modal-backdrop" onClick={() => close(false)}>
            <div className="modal" role="dialog" aria-modal="true" onClick={e => e.stopPropagation()}>
                <h3>{r.title}</h3>
                <div className="modal-body">{r.message}</div>
                <div className="modal-actions">
                    {/* Enter must not confirm a destructive action by accident. */}
                    <button autoFocus={r.danger} onClick={() => close(false)}>取消</button>
                    <button className={r.danger ? 'danger' : 'primary'} autoFocus={!r.danger} onClick={() => close(true)}>{r.confirm}</button>
                </div>
            </div>
        </div>
    )
}

// Dialog is a modal with its own buttons. Escape and the backdrop call
// onClose unless busy.
export function Dialog(props: {title: ReactNode, onClose: () => void, busy?: boolean, width?: number, actions: ReactNode, children: ReactNode}) {
    const {onClose, busy} = props
    useEffect(() => {
        const onKey = (e: KeyboardEvent) => {
            if (e.key === 'Escape' && !busy) onClose()
        }
        window.addEventListener('keydown', onKey)
        return () => window.removeEventListener('keydown', onKey)
    }, [onClose, busy])
    return (
        <div className="modal-backdrop" onClick={() => !busy && onClose()}>
            <div className="modal" style={props.width ? {width: props.width} : undefined} role="dialog" aria-modal="true"
                 onClick={e => e.stopPropagation()}>
                <h3>{props.title}</h3>
                <div className="modal-body">{props.children}</div>
                <div className="modal-actions">{props.actions}</div>
            </div>
        </div>
    )
}

// Modal shows content in a dialog that Escape, the backdrop or 关闭 dismisses.
export function Modal(props: {title: string, onClose: () => void, children: ReactNode, wide?: boolean}) {
    return (
        <Dialog title={props.title} onClose={props.onClose} width={props.wide ? 700 : undefined}
                actions={<button className="primary" autoFocus onClick={props.onClose}>关闭</button>}>
            {props.children}
        </Dialog>
    )
}

export function Spinner() {
    return <span className="spinner" aria-hidden="true"/>
}

// useClock re-renders every second so elapsed times stay current.
export function useClock(active: boolean) {
    const [, setTick] = useState(0)
    useEffect(() => {
        if (!active) return
        const id = window.setInterval(() => setTick(t => t + 1), 1000)
        return () => window.clearInterval(id)
    }, [active])
}

// formatElapsed shows a duration like "1 分 12 秒".
export function formatElapsed(ms: number): string {
    const total = Math.max(0, Math.round(ms / 1000))
    const m = Math.floor(total / 60)
    const s = total % 60
    return m > 0 ? `${m} 分 ${s} 秒` : `${s} 秒`
}

// formatClock shows a duration like "1:12" for the status bar.
export function formatClock(ms: number): string {
    const total = Math.max(0, Math.floor(ms / 1000))
    return `${Math.floor(total / 60)}:${(total % 60).toString().padStart(2, '0')}`
}

// formatTime shows a moment like "10-22 10:21".
export function formatTime(unixMs: number): string {
    if (!unixMs) return '—'
    const d = new Date(unixMs)
    const two = (n: number) => n.toString().padStart(2, '0')
    return `${two(d.getMonth() + 1)}-${two(d.getDate())} ${two(d.getHours())}:${two(d.getMinutes())}`
}

export function Check() {
    return (
        <svg width="9" height="9" viewBox="0 0 12 12" aria-hidden="true">
            <path d="M2 6.5l2.5 2.5L10 3.5" stroke="#fff" strokeWidth="2" fill="none"/>
        </svg>
    )
}
