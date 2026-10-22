import {ReactNode, useCallback, useState} from 'react'
import {api, kindLabels, Server, TaskKind, TaskOptions} from '../api'
import {CredentialRequest, CredentialsDialog} from './dialogs'

type Ask = {
    title: string
    requests: CredentialRequest[]
    confirm: string
    warning?: ReactNode
    run: (values: string[]) => Promise<void>
}

async function request(sv: Server, kind: TaskKind, role?: string): Promise<CredentialRequest | null> {
    const need = await api.secretNeeded(sv.id, kind)
    return need ? {server: sv, need, role} : null
}

function secretOf(r: CredentialRequest | null, value: string) {
    return {password: r?.need === 'password' ? value : '', passphrase: r?.need === 'passphrase' ? value : ''}
}

// useLaunch starts a task after asking for whatever secrets its servers need
// (a relay with a front needs two). dialog must be rendered by the caller.
export function useLaunch(onError: (text: string) => void) {
    const [ask, setAsk] = useState<Ask | null>(null)

    const launch = useCallback(async (sv: Server, kind: TaskKind, opts: TaskOptions, confirm: string, front?: Server, warning?: ReactNode) => {
        try {
            const main = await request(sv, kind, front ? '中转服务器' : undefined)
            const frontRequest = front ? await request(front, kind, '前置机') : null
            const start = async (values: string[]) => {
                let i = 0
                const own = secretOf(main, main ? values[i++] : '')
                const theirs = secretOf(frontRequest, frontRequest ? values[i++] : '')
                await api.startTask(kind, sv.id, own.password, own.passphrase,
                    {...opts, frontPassword: theirs.password, frontPassphrase: theirs.passphrase})
            }
            const requests = [main, frontRequest].filter((r): r is CredentialRequest => r !== null)
            if (requests.length === 0) {
                await start([])
            } else {
                setAsk({title: kindLabels[kind], requests, confirm, warning, run: start})
            }
        } catch (e) {
            onError(e instanceof Error ? e.message : String(e))
        }
    }, [onError])

    const dialog = ask && (
        <CredentialsDialog title={ask.title} requests={ask.requests} confirm={ask.confirm} warning={ask.warning}
                           onClose={() => setAsk(null)} onSubmit={ask.run}/>
    )
    return {launch, dialog}
}
