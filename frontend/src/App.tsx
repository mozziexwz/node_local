import {ReactNode, useCallback, useEffect, useRef, useState} from 'react'
import {api, errorText, kindLabels, on, Overview, TaskEvent} from './api'
import {Banner, Confirm, ConfirmRequest, formatClock, Spinner, useClock} from './components'
import {Records} from './pages/Records'
import {Servers} from './pages/Servers'
import {Settings} from './pages/Settings'

type Page = 'servers' | 'records' | 'settings'

const pages: {id: Page, label: string}[] = [
    {id: 'servers', label: '服务器'},
    {id: 'records', label: '任务记录'},
    {id: 'settings', label: '设置与关于'},
]

const serverIcon = (
    <svg className="tab-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8"
         strokeLinecap="round" aria-hidden="true">
        <rect x="3" y="4" width="18" height="7" rx="2"/>
        <rect x="3" y="13" width="18" height="7" rx="2"/>
        <path d="M7 7.5h.01M7 16.5h.01"/>
    </svg>
)

const homepage = 'https://msboost.de/'

export type AskConfirm = (title: string, message: ReactNode, confirm: string, danger?: boolean) => Promise<boolean>

// StatusBar shows the operation in progress and where the launcher is.
function StatusBar(props: {overview: Overview | null, tasks: Record<string, TaskEvent>}) {
    const running = Object.values(props.tasks).filter(t => t.state === 'running' && t.kind !== 'status')
    useClock(running.length > 0)
    let left: ReactNode = '就绪'
    if (running.length === 1) {
        const t = running[0]
        const sv = props.overview?.servers?.find(s => s.id === t.serverId)
        left = <><Spinner/>正在{kindLabels[t.kind]} <b>{sv?.label}</b> · 已用 {formatClock(Date.now() - t.started)}</>
    } else if (running.length > 1) {
        left = <><Spinner/>{running.length} 个操作正在进行</>
    }
    const la = props.overview?.launcher
    const right = !props.overview ? '' : la ? (la.beside ? '启动器在同一文件夹' : `启动器：${la.dir}`) : '还没找到启动器'
    return (
        <footer className="statusbar">
            <div className="status-left">{left}</div>
            <div className="status-right">{right}</div>
        </footer>
    )
}

export default function App() {
    const [page, setPage] = useState<Page>('servers')
    const [overview, setOverview] = useState<Overview | null>(null)
    const [loadError, setLoadError] = useState('')
    // The latest event per server; finished ones stay until dismissed.
    const [tasks, setTasks] = useState<Record<string, TaskEvent>>({})
    const [confirmRequest, setConfirmRequest] = useState<ConfirmRequest | null>(null)
    const refreshing = useRef<Promise<void> | null>(null)
    const refreshAgain = useRef(false)

    // refresh reloads the overview; calls made while one runs are merged into
    // a single follow-up.
    const refresh = useCallback((): Promise<void> => {
        if (refreshing.current) {
            refreshAgain.current = true
            return refreshing.current
        }
        const run = async () => {
            do {
                refreshAgain.current = false
                try {
                    const o = await api.overview()
                    setOverview(o)
                    setTasks(t => {
                        const next = {...t}
                        for (const e of o.running ?? []) {
                            if (!next[e.serverId] || next[e.serverId].id !== e.id) next[e.serverId] = e
                        }
                        return next
                    })
                    setLoadError('')
                } catch (e) {
                    setLoadError(errorText(e))
                }
            } while (refreshAgain.current)
            refreshing.current = null
        }
        refreshing.current = run()
        return refreshing.current
    }, [])

    useEffect(() => {
        refresh()
        const off = on.task(e => {
            setTasks(t => ({...t, [e.serverId]: e}))
            if (e.state !== 'running') refresh()
        })
        const onFocus = () => refresh()
        window.addEventListener('focus', onFocus)
        return () => {
            off()
            window.removeEventListener('focus', onFocus)
        }
    }, [refresh])

    const confirm: AskConfirm = useCallback((title, message, confirmLabel, danger = false) =>
        new Promise<boolean>(resolve => setConfirmRequest({title, message, confirm: confirmLabel, danger, resolve})), [])

    const dismissTask = useCallback((serverId: string) => setTasks(t => {
        const next = {...t}
        delete next[serverId]
        return next
    }), [])

    const go = (p: Page) => {
        setPage(p)
        refresh()
    }

    let content: ReactNode = null
    if (overview) {
        switch (page) {
            case 'servers':
                content = <Servers overview={overview} tasks={tasks} refresh={refresh} confirm={confirm} dismissTask={dismissTask}/>
                break
            case 'records':
                content = <Records overview={overview} confirm={confirm}/>
                break
            case 'settings':
                content = <Settings overview={overview} refresh={refresh} confirm={confirm}/>
                break
        }
    }

    return (
        <div className="shell">
            <nav className="tabs">
                {pages.map(p => (
                    <button key={p.id} className={'tab' + (p.id === page ? ' active' : '')}
                            aria-current={p.id === page ? 'page' : undefined} onClick={() => go(p.id)}>
                        {p.id === 'servers' && serverIcon}{p.label}
                    </button>
                ))}
                <div className="tabs-right">
                    <span>{overview?.version}</span>
                    <button className="link home-link" onClick={() => api.openURL(homepage)}>MSBOOST</button>
                </div>
            </nav>

            <main className="content">
                {(overview?.warnings ?? []).map(w => <Banner key={w} kind="warn">{w}</Banner>)}
                {loadError && <Banner kind="error">无法读取程序状态：{loadError}</Banner>}
                {overview ? content : !loadError && <div className="loading"><Spinner/>正在加载…</div>}
            </main>

            <StatusBar overview={overview} tasks={tasks}/>
            <Confirm request={confirmRequest} onDone={() => setConfirmRequest(null)}/>
        </div>
    )
}
