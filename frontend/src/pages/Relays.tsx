import {useCallback, useEffect, useRef, useState} from 'react'
import {api, busyEvent, ConfigItem, errorText, Overview, RelayRow, Server, TaskEvent} from '../api'
import type {AskConfirm} from '../App'
import {Banner, Card, Spinner} from '../components'
import {RateDialog, UninstallDialog} from './dialogs'
import {useLaunch} from './launch'

const sourceLabels: Record<string, string> = {
    self: '自备中转', friend: '朋友中转', front: '前置机', website: '网站', code: '网站中转码', unknown: '未记录',
}
const stateLabels: Record<string, string> = {running: '运行中', paused: '已暂停', stopped: '已停止'}

type Message = {kind: 'error' | 'info', text: string}

export function Relays(props: {
    overview: Overview
    tasks: Record<string, TaskEvent>
    confirm: AskConfirm
    focus: string
    dismissTask: (serverId: string) => void
}) {
    const list = props.overview.servers ?? []
    const first = props.focus || list[0]?.id || ''
    const [configs, setConfigs] = useState<ConfigItem[]>([])
    const [relayId, setRelayId] = useState(first)
    const [configName, setConfigName] = useState('')
    const [frontId, setFrontId] = useState('')
    const [remark, setRemark] = useState('自备中转')
    const [rate, setRate] = useState('5')
    const [manageId, setManageId] = useState(first)
    const [rows, setRows] = useState<Record<string, RelayRow[]>>({})
    const [rateRow, setRateRow] = useState<RelayRow | null>(null)
    const [message, setMessage] = useState<Message | null>(null)
    const showError = useCallback((text: string) => setMessage({kind: 'error', text}), [])
    const {launch, dialog} = useLaunch(showError)
    const mounted = useRef(Date.now())
    const handled = useRef(new Set<string>())

    useEffect(() => {
        api.configs().then(c => {
            setConfigs(c)
            setConfigName(n => c.some(x => x.name === n) ? n : c[0]?.name ?? '')
        }).catch(e => showError(errorText(e)))
    }, [props.overview, showError])

    const server = (id: string) => list.find(s => s.id === id)
    const manageServer = server(manageId)

    // Keep each list; after a change made here, read the list again (the
    // secret is still remembered, so no password is asked).
    useEffect(() => {
        for (const t of Object.values(props.tasks)) {
            if (t.state === 'running' || handled.current.has(t.id)) continue
            handled.current.add(t.id)
            if (t.kind === 'relay-list' && t.state === 'succeeded') {
                setRows(r => ({...r, [t.serverId]: t.outcome?.relays ?? []}))
            }
            if ((t.finished ?? 0) < mounted.current) continue
            if (t.kind === 'relay-manage' || t.kind === 'relay-uninstall') {
                setMessage({kind: t.state === 'succeeded' ? 'info' : 'error', text: t.outcome?.message ?? ''})
                const sv = server(t.serverId)
                if (t.state === 'succeeded' && sv) launch(sv, 'relay-list', {}, '读取列表')
            }
            if (t.kind === 'relay-uninstall-preview' && t.state === 'succeeded' && !(t.outcome?.items?.length)) {
                setMessage({kind: 'info', text: t.outcome?.message ?? ''})
            }
        }
    })

    const relay = server(relayId)
    const front = frontId ? server(frontId) : undefined
    const config = configs.find(c => c.name === configName)
    const created = relay && props.tasks[relay.id]?.kind === 'relay' ? props.tasks[relay.id] : undefined
    const rateNumber = Number(rate)
    let problem = ''
    if (!relay) problem = '先在「服务器」页添加中转服务器'
    else if (!config) problem = '配置文件夹里还没有节点配置'
    else if (config.entry.startsWith(relay.host + ':') || config.entry.startsWith('[' + relay.host + ']:')) problem = '中转服务器就是这份配置的节点本身'
    else if (front && front.id === relay.id) problem = '前置机必须是另一台服务器'
    else if (!Number.isInteger(rateNumber) || rateNumber < 1 || rateNumber > 10) problem = '带宽为 1–10 Mbps'

    const create = () => {
        if (!relay || problem) return
        setMessage(null)
        props.dismissTask(relay.id)
        launch(relay, 'relay', {configName, frontId: front?.id, remark: remark.trim(), rateMbps: rateNumber}, '创建中转', front)
    }

    const busy = manageServer ? busyEvent(props.tasks, manageServer.id) : undefined
    const latest = manageServer ? props.tasks[manageServer.id] : undefined
    const shown = manageServer ? rows[manageServer.id] : undefined
    const preview = latest?.kind === 'relay-uninstall-preview' && latest.state === 'succeeded' && (latest.outcome?.items?.length ?? 0) > 0

    const readList = () => {
        if (!manageServer) return
        setMessage(null)
        launch(manageServer, 'relay-list', {}, '读取列表')
    }
    const manage = (row: RelayRow, action: string, rateMbps?: number) =>
        manageServer && launch(manageServer, 'relay-manage', {action, items: [row.id], rateMbps}, '确定')
    const remove = async (row: RelayRow) => {
        const consequence = {
            friend: '删除后朋友就连不上了，记得告诉他。',
            self: '用这条中转生成的配置会失效。',
            front: '经过这台前置机的配置会失效。',
            website: '这是网站建的中转，删除后用它的人会连不上。',
            code: '这是网站中转码建的中转，删除后用它的人会连不上。',
        }[row.source] ?? '删除后经过它的连接都会断开。'
        if (await props.confirm('删除中转', `删除端口 ${row.port} 上的中转（${sourceLabels[row.source]}${row.remark ? ' · ' + row.remark : ''}）？${consequence}`, '删除', true)) {
            manage(row, 'delete')
        }
    }
    const copyEntry = async (row: RelayRow) => {
        await api.copy(`MSBOOST 中转入口 ${row.entry} → ${row.target}`)
        setMessage({kind: 'info', text: `已复制端口 ${row.port} 的中转入口，发给朋友即可。`})
    }

    return (
        <div className="page fill">
            {message?.text && <Banner kind={message.kind} onClose={() => setMessage(null)}>{message.text}</Banner>}

            <Card title="新建自备中转" actions={
                <button className="primary" disabled={!!problem || !!(relay && busyEvent(props.tasks, relay.id)) || !!(front && busyEvent(props.tasks, front.id))}
                        onClick={create}>创建中转</button>
            }>
                <div className="relay-form">
                    <label>中转服务器</label>
                    <select value={relayId} onChange={e => setRelayId(e.target.value)}>
                        {list.map(s => <option key={s.id} value={s.id}>{s.label}（{s.host}）</option>)}
                    </select>
                    <label>目标节点</label>
                    <select value={configName} onChange={e => setConfigName(e.target.value)}>
                        {configs.length === 0 && <option value="">没有节点配置</option>}
                        {configs.map(c => <option key={c.name} value={c.name}>{c.name}（{c.entry}）</option>)}
                    </select>
                    <label>前置机</label>
                    <select value={frontId} onChange={e => setFrontId(e.target.value)}>
                        <option value="">不使用</option>
                        {list.filter(s => s.id !== relayId).map(s => <option key={s.id} value={s.id}>{s.label}（{s.host}）</option>)}
                    </select>
                    <label>备注</label>
                    <input value={remark} maxLength={60} onChange={e => setRemark(e.target.value)}/>
                    <label>带宽</label>
                    <div className="rate"><input value={rate} inputMode="numeric" onChange={e => setRate(e.target.value.replace(/\D/g, ''))}/><span className="muted">Mbps，上下行各，1–10</span></div>
                    <span className="hint wide">{problem || `建好后生成「${remark.trim() || '自备中转'}.json」；帮朋友建中转请用「朋友中转」页`}</span>
                </div>
                {created && <Created task={created} done={() => props.dismissTask(created.serverId)}/>}
            </Card>

            <Card className="fill" title={manageServer ? `${manageServer.label} 上的中转${shown ? `（${shown.length}）` : ''}` : '中转管理'} actions={<>
                <select value={manageId} onChange={e => setManageId(e.target.value)} style={{width: 'auto'}}>
                    {list.map(s => <option key={s.id} value={s.id}>{s.label}</option>)}
                </select>
                <button className="small" disabled={!manageServer || !!busy} onClick={readList}>
                    {busy?.kind === 'relay-list' && <Spinner/>}{shown ? '刷新' : '读取列表'}
                </button>
                <button className="small danger-text" disabled={!manageServer || !!busy}
                        onClick={() => manageServer && launch(manageServer, 'relay-uninstall-preview', {}, '读取卸载清单')}>全部卸载…</button>
            </>}>
                {!shown && <p className="muted">点「读取列表」查看这台服务器上的中转。需要输入这台服务器的密码，之后 10 分钟内的管理操作不用再输。</p>}
                {shown && shown.length === 0 && <p className="empty">这台服务器上没有中转。</p>}
                {shown && shown.length > 0 && (
                    <div className="table-scroll grow-table">
                        <table className="list relays">
                            <thead><tr><th>入口端口</th><th>目标</th><th>带宽</th><th>状态</th><th>连接</th><th>来源</th><th/></tr></thead>
                            <tbody>
                                {shown.map(r => (
                                    <tr key={r.id}>
                                        <td>{r.port}</td>
                                        <td>{r.target}</td>
                                        <td>{r.rateMbps} Mbps</td>
                                        <td className={r.state === 'running' ? 'ok' : 'muted'}>{r.state === 'running' && <span className="dot"/>}{stateLabels[r.state] ?? r.state}</td>
                                        <td>{r.connections}</td>
                                        <td title={r.remark}>{sourceLabels[r.source] ?? r.source}{r.remark && r.remark !== sourceLabels[r.source] && ` · ${r.remark}`}</td>
                                        <td className="actions">
                                            {r.source === 'friend' && <button className="small" onClick={() => copyEntry(r)}>复制入口</button>}
                                            {r.state === 'paused'
                                                ? <button className="small" disabled={!!busy} onClick={() => manage(r, 'resume')}>恢复</button>
                                                : <button className="small" disabled={!!busy} onClick={() => manage(r, 'pause')}>暂停</button>}
                                            <button className="small" disabled={!!busy} onClick={() => setRateRow(r)}>限速</button>
                                            <button className="small danger-text" disabled={!!busy} onClick={() => remove(r)}>删除</button>
                                        </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                )}
                <p className="hint" style={{marginTop: 'auto'}}>来源和备注记在这台电脑上；网站建的中转显示网站的记录。删除只删中转本身，不动其他软件。</p>
            </Card>

            {dialog}
            {rateRow && <RateDialog row={rateRow} onClose={() => setRateRow(null)} onSubmit={async mbps => {
                await manage(rateRow, 'rate', mbps)
            }}/>}
            {preview && manageServer && latest && <UninstallDialog server={manageServer} task={latest} onClose={() => props.dismissTask(manageServer.id)}/>}
        </div>
    )
}

// Created shows the self relay task in progress or its result.
function Created(props: {task: TaskEvent, done: () => void}) {
    const {task} = props
    const o = task.outcome ?? {}
    if (task.state === 'running') {
        return <Banner kind="info"><Spinner/> 正在建中转{task.frontId ? '（先中转服务器，再前置机）' : ''}，通常一分钟左右…</Banner>
    }
    if (task.state !== 'succeeded') {
        return (
            <Banner kind="error" onClose={props.done}>
                新建自备中转没有完成：{o.message}{o.nextStep && <><br/>{o.nextStep}</>}
            </Banner>
        )
    }
    return (
        <Banner kind="info" onClose={props.done}>
            中转已建好，入口 {o.entry}{o.health?.publicTCP === 'unreachable' && '（本机暂时连不上入口，请检查服务商安全组是否放行这个端口）'}。
            配置已保存为 {o.configPath}，在启动器的「节点配置」里选中它即可使用。
        </Banner>
    )
}
