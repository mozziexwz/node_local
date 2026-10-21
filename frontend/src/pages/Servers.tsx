import {ReactNode, useEffect, useState} from 'react'
import {api, busyLabels, errorText, itemKind, keyType, kindLabels, Overview, SecretKind, Server, stepLabels, TaskEvent, TaskKind} from '../api'
import type {AskConfirm} from '../App'
import {Banner, Card, Check, formatElapsed, formatTime, Spinner, useClock} from '../components'
import {FingerprintDialog, SecretDialog, ServerDialog, UninstallDialog} from './dialogs'

type Props = {
    overview: Overview
    tasks: Record<string, TaskEvent>
    refresh: () => Promise<void>
    confirm: AskConfirm
    dismissTask: (serverId: string) => void
}

// Kinds whose result gets its own card instead of a banner.
const resultKinds: TaskKind[] = ['deploy', 'upgrade', 'fetch', 'uninstall']

export function Servers(props: Props) {
    const list = props.overview.servers ?? []
    const [selected, setSelected] = useState(list[0]?.id ?? '')
    const [editing, setEditing] = useState<Server | 'new' | null>(null)
    const [secret, setSecret] = useState<{server: Server, kind: TaskKind, need: SecretKind} | null>(null)
    const [rekey, setRekey] = useState<Server | null>(null)
    const [error, setError] = useState('')

    useEffect(() => {
        if (!list.some(s => s.id === selected)) setSelected(list[0]?.id ?? '')
    }, [list, selected])

    // A successful status check needs no result card.
    useEffect(() => {
        for (const t of Object.values(props.tasks)) {
            if (t.kind === 'status' && t.state === 'succeeded') props.dismissTask(t.serverId)
        }
    }, [props.tasks, props.dismissTask])

    const server = list.find(s => s.id === selected)
    const task = server ? props.tasks[server.id] : undefined

    const start = async (sv: Server, kind: TaskKind) => {
        setError('')
        try {
            const need = await api.secretNeeded(sv.id)
            if (need !== '') {
                setSecret({server: sv, kind, need})
                return
            }
            if (kind === 'deploy' && sv.node?.installed && !await props.confirm('重新部署',
                `会生成新配置，${sv.configFile || '旧配置'} 等旧配置会立即失效。确定重新部署吗？`, '重新部署', true)) {
                return
            }
            await api.startTask(kind, sv.id, '', '')
        } catch (e) {
            setError(errorText(e))
        }
    }

    const remove = async (sv: Server) => {
        if (!await props.confirm('移除服务器', `从列表移除“${sv.label}”？只删除本程序里的记录，服务器上的节点和配置文件都不受影响。`, '移除', true)) return
        try {
            await api.removeServer(sv.id)
            props.dismissTask(sv.id)
            await props.refresh()
        } catch (e) {
            setError(errorText(e))
        }
    }

    // importToLauncher copies a configuration saved before the launcher was
    // known; it asks where the launcher is first if needed.
    const importToLauncher = async (sv: Server) => {
        setError('')
        try {
            if (!props.overview.launcher && !await api.pickLauncher()) return
            await api.copyToLauncher(sv.id)
            await props.refresh()
        } catch (e) {
            setError(errorText(e))
        }
    }

    let panel: ReactNode
    if (!server) {
        panel = <Welcome onAdd={() => setEditing('new')}/>
    } else if (task && task.state === 'running' && (task.kind === 'deploy' || task.kind === 'upgrade')) {
        panel = <Progress server={server} task={task}/>
    } else if (task && task.state === 'running' && task.kind === 'uninstall') {
        panel = <Card className="fill" title={`正在卸载 · ${server.label}`}><p className="muted"><Spinner/> 正在删除节点组件…</p></Card>
    } else if (task && task.state !== 'running' && resultKinds.includes(task.kind)) {
        panel = <Result server={server} task={task} overview={props.overview} done={() => props.dismissTask(server.id)}
                        retry={() => {
                            props.dismissTask(server.id)
                            start(server, task.kind === 'uninstall' ? 'uninstall-preview' : task.kind)
                        }}
                        importToLauncher={() => importToLauncher(server)} rekey={() => setRekey(server)}/>
    } else {
        panel = <Detail server={server} task={task} onStart={kind => start(server, kind)} onEdit={() => setEditing(server)}
                        onRemove={() => remove(server)} importToLauncher={() => importToLauncher(server)}
                        dismiss={() => props.dismissTask(server.id)} rekey={() => setRekey(server)}/>
    }

    const preview = task && task.kind === 'uninstall-preview' && task.state === 'succeeded' && (task.outcome?.items?.length ?? 0) > 0

    return (
        <div className="split">
            <div className="col">
                <Card className="fill" title="服务器" actions={<button className="small primary" onClick={() => setEditing('new')}>添加</button>}>
                    {list.length === 0 && <p className="muted small">还没有服务器。</p>}
                    <ul className="server-list">
                        {list.map(sv => (
                            <li key={sv.id} className={sv.id === selected ? 'sel' : ''} onClick={() => setSelected(sv.id)}>
                                <div className="top"><span className="name">{sv.label}</span><ServerTag server={sv} task={props.tasks[sv.id]}/></div>
                                <span className="addr">{sv.host}{sv.port !== 22 && `:${sv.port}`}</span>
                            </li>
                        ))}
                    </ul>
                    <p className="list-foot">只保存地址和指纹，不保存密码。</p>
                </Card>
            </div>
            <div className="col">
                {error && <Banner kind="error" onClose={() => setError('')}>{error}</Banner>}
                {panel}
            </div>

            {editing && <ServerDialog server={editing === 'new' ? undefined : editing} onClose={() => setEditing(null)}
                                      onSaved={async sv => {
                                          setEditing(null)
                                          await props.refresh()
                                          setSelected(sv.id)
                                      }}/>}
            {secret && <SecretDialog server={secret.server} kind={secret.kind} need={secret.need} onClose={() => setSecret(null)}/>}
            {rekey && <FingerprintDialog server={rekey} onClose={() => setRekey(null)} onReplaced={async () => {
                setRekey(null)
                props.dismissTask(rekey.id)
                await props.refresh()
            }}/>}
            {preview && server && task && <UninstallDialog server={server} task={task} onClose={() => props.dismissTask(server.id)}/>}
        </div>
    )
}

function ServerTag(props: {server: Server, task?: TaskEvent}) {
    const {server, task} = props
    if (task?.state === 'running') return <span className="tag busy"><Spinner/>{busyLabels[task.kind]}</span>
    const node = server.node
    if (!node) return <span className="tag">未检查</span>
    if (!node.installed) return <span className="tag">未部署</span>
    if (node.service === 'stopped') return <span className="tag warn"><span className="dot"/>节点已停止</span>
    return <span className="tag ok"><span className="dot"/>节点运行中</span>
}

function Welcome(props: {onAdd: () => void}) {
    return (
        <Card className="fill" title="开始使用">
            <p>MSBOOST 本地版在这台电脑上直接用 SSH 登录你的 VPS，部署 MSBOOST 节点。SSH 密码和私钥不经过网站，也不保存到硬盘。</p>
            <ol className="report">
                <li>准备一台 Debian 11–13 的 VPS，root 能用密码或私钥登录。</li>
                <li>点「添加」，填写 IP 和 SSH 端口，读取并核对服务器指纹。</li>
                <li>点「部署节点」，输入 root 密码，等几分钟。</li>
                <li>配置会存进启动器的 configs 文件夹，在启动器里选中它就能加速。</li>
            </ol>
            <div className="row-actions"><button className="primary" onClick={props.onAdd}>添加服务器</button></div>
        </Card>
    )
}

const bbrLabels: Record<string, string> = {
    enabled: 'BBR 已开启', off: 'BBR 未开启', unavailable: '内核不支持 BBR', review_required: 'BBR 需要人工检查', partial: 'BBR 部分开启',
}

function NodeLine(props: {server: Server}) {
    const node = props.server.node
    if (!node) return <span className="muted">还没检查，点「刷新状态」看看</span>
    const facts: string[] = []
    if (node.installed) {
        if (node.bbr) facts.push(bbrLabels[node.bbr] ?? node.bbr)
        if (node.engine) facts.push('引擎 ' + node.engine)
        if (node.publicTCP) facts.push(node.publicTCP === 'reachable' ? '本机到节点端口可达' : '本机到节点端口不可达')
    } else if (node.os) {
        facts.push(node.os)
    }
    facts.push(formatTime(node.checkedAt) + ' 检查')
    return (
        <>
            {!node.installed ? <span>未部署</span>
                : node.service === 'stopped' ? <span className="bad"><span className="dot"/>已停止</span>
                    : <span className="ok"><span className="dot"/>运行中</span>}
            <span className="muted"> · {facts.join(' · ')}</span>
        </>
    )
}

function Detail(props: {
    server: Server
    task?: TaskEvent
    onStart: (kind: TaskKind) => void
    onEdit: () => void
    onRemove: () => void
    importToLauncher: () => void
    dismiss: () => void
    rekey: () => void
}) {
    const {server, task} = props
    const busy = task?.state === 'running'
    const installed = !!server.node?.installed
    const failed = task && !busy && task.state !== 'succeeded'
    const empty = task?.kind === 'uninstall-preview' && task.state === 'succeeded' && !(task.outcome?.items?.length)
    const act = (kind: TaskKind, label: string, primary = false) => (
        <button className={primary ? 'primary' : ''} disabled={busy} onClick={() => props.onStart(kind)}>
            {busy && task?.kind === kind && <Spinner/>}{label}
        </button>
    )
    return (
        <Card title={server.label} actions={<>
            <button className="small" disabled={busy} onClick={props.onEdit}>编辑</button>
            <button className="small danger-text" disabled={busy} onClick={props.onRemove}>移除</button>
        </>}>
            {failed && (
                <Banner kind={task.state === 'cancelled' ? 'warn' : 'error'} onClose={props.dismiss}>
                    {kindLabels[task.kind]}没有完成：{task.outcome?.message}
                    {task.outcome?.nextStep && <><br/>{task.outcome.nextStep}</>}
                    {task.outcome?.errorCode === 'ssh_host_changed' &&
                        <><br/><button className="link" onClick={props.rekey}>重新读取指纹…</button></>}
                </Banner>
            )}
            {empty && <Banner kind="info" onClose={props.dismiss}>{task.outcome?.message}</Banner>}
            <dl className="facts">
                <dt>地址</dt>
                <dd>{server.host}:{server.port}<span className="muted">（root，{server.authMethod === 'key' ? '私钥登录' : '密码登录'}）</span></dd>
                <dt>主机指纹</dt>
                <dd><span className="fp">{server.fingerprint}</span> <span className="hint">{keyType(server.algorithm)} · {formatTime(server.trustedAt)} 信任</span></dd>
                <dt>节点</dt>
                <dd><NodeLine server={server}/></dd>
                <dt>配置文件</dt>
                <dd>
                    {!server.configFile ? <span className="muted">还没有</span>
                        : !server.configPath ? <>{server.configFile} <span className="hint">文件已经不在了，可以用「取回配置」重新保存</span></>
                            : server.inLauncher ? <>{server.configFile} <span className="hint">在启动器的 configs 文件夹，可直接使用</span></>
                                : <>{server.configFile} <span className="hint">在本程序旁边的 configs 文件夹</span>
                                    {' '}<button className="link" onClick={props.importToLauncher}>导入到启动器…</button></>}
                </dd>
            </dl>
            <div className="row-actions">
                {installed ? <>
                    {act('status', '刷新状态')}
                    {act('fetch', '取回配置')}
                    {act('upgrade', '升级节点')}
                    <button disabled={busy} onClick={() => props.onStart('deploy')}>重新部署…</button>
                    <button className="danger-text" style={{marginLeft: 'auto'}} disabled={busy}
                            onClick={() => props.onStart('uninstall-preview')}>
                        {busy && task?.kind === 'uninstall-preview' && <Spinner/>}卸载…
                    </button>
                </> : <>
                    {act('deploy', '部署节点', true)}
                    {act('status', '刷新状态')}
                </>}
            </div>
            {installed ? (
                <div className="explain">
                    <b>升级节点</b><span>换新版安装脚本，用户名、密码、端口不变，原配置继续能用。</span>
                    <b>重新部署</b><span>生成新配置，旧配置立即失效。</span>
                    <b>取回配置</b><span>从节点读回当前配置，不改动服务器；部署时断线没拿到配置就用它。</span>
                </div>
            ) : (
                <p className="hint">部署会安装 MSBOOST 节点并开启 BBR，通常 2–5 分钟。服务器需要 Debian 11–13，并能访问 GitHub。</p>
            )}
        </Card>
    )
}

function Progress(props: {server: Server, task: TaskEvent}) {
    const {server, task} = props
    useClock(true)
    const current = Math.max(task.step, 1)
    const total = stepLabels.length
    return (
        <Card className="fill" title={`正在${kindLabels[task.kind]} · ${server.label}`}
              actions={<span className="muted small">已用 {formatElapsed(Date.now() - task.started)}，通常 2–5 分钟</span>}>
            <div className="progress"><span style={{width: `${Math.round((current - 0.5) / total * 100)}%`}}/></div>
            <ul className="steps">
                {stepLabels.map((label, i) => {
                    const n = i + 1
                    const state = n < current ? 'done' : n === current ? 'now' : ''
                    return (
                        <li key={n} className={state}>
                            <span className={'ic ' + (n < current ? 'check' : n === current ? '' : 'todo')}>
                                {n < current ? <Check/> : n === current ? <Spinner/> : null}
                            </span>
                            {label}
                        </li>
                    )
                })}
            </ul>
            <div className="notice" style={{marginTop: 'auto'}}>
                取消只会断开这台电脑的连接。服务器上的安装会继续跑完，之后可以用「取回配置」拿到结果。
            </div>
            <div className="row-actions">
                <button className="danger-text" style={{marginLeft: 'auto'}} onClick={() => api.cancelTask(task.id)}>取消</button>
            </div>
        </Card>
    )
}

const doneTitles: Partial<Record<TaskKind, string>> = {deploy: '部署完成', upgrade: '升级完成', fetch: '已取回配置', uninstall: '卸载完成'}

function Result(props: {
    server: Server
    task: TaskEvent
    overview: Overview
    done: () => void
    retry: () => void
    importToLauncher: () => void
    rekey: () => void
}) {
    const {server, task} = props
    const o = task.outcome ?? {}
    const ok = task.state === 'succeeded'
    const title = ok ? doneTitles[task.kind] : task.state === 'cancelled' ? `已取消${kindLabels[task.kind]}` : `${kindLabels[task.kind]}没有完成`
    const launcher = props.overview.launcher
    const port = o.node?.entry?.split(':').pop()
    const h = o.health
    return (
        <Card className="fill" title={`${title} · ${server.label}`}
              actions={<span className="muted small">用时 {formatElapsed((task.finished ?? Date.now()) - task.started)}</span>}>
            {ok && o.configPath && (
                <Banner kind="info">
                    配置已保存为 {o.configPath}。
                    {server.inLauncher
                        ? (launcher?.beside ? '启动器在同一文件夹，' : '已存进启动器的 configs 文件夹，') + '打开启动器的「节点配置」就能看到。'
                        : '还不知道启动器在哪里：点「导入到启动器…」选一次启动器的位置，以后会直接存进去。'}
                </Banner>
            )}
            {ok && task.kind === 'uninstall' && <Banner kind="info">{o.message}</Banner>}
            {!ok && (
                <Banner kind={task.state === 'cancelled' ? 'warn' : 'error'}>
                    {o.message}{o.errorCode && <span className="diag">（诊断编号 {o.errorCode}）</span>}
                </Banner>
            )}
            {!ok && o.nextStep && <p className="muted">{o.nextStep}</p>}
            {ok && h && (
                <dl className="check">
                    <dt>节点服务</dt>
                    <dd className={h.service === 'running' ? 'ok' : 'bad'}>{h.service === 'running' ? '运行中' : '已停止'}</dd>
                    {h.localSelfTest === 'passed' && <><dt>本机自测</dt><dd className="ok">通过</dd></>}
                    <dt>本机到节点端口</dt>
                    {h.publicTCP === 'reachable'
                        ? <dd className="ok">可达 {port && <span className="muted">（TCP {port}）</span>}</dd>
                        : <dd className="bad">不可达 <span className="muted">检查服务商安全组是否放行{port ? ` TCP ${port}` : '这个端口'}</span></dd>}
                    {h.bbr && <><dt>BBR</dt><dd>{(bbrLabels[h.bbr] ?? h.bbr).replace('BBR ', '')}</dd></>}
                    <dt>游戏连接</dt>
                    <dd><span className="muted">还没测：在启动器选中这份配置，看「网络检测」</span></dd>
                </dl>
            )}
            {ok && task.kind === 'uninstall' && o.items && (
                <ul className="paths">{o.items.map(i => <li key={i.path}><span className="k">{itemKind(i.kind)}</span>{i.path}</li>)}</ul>
            )}
            <div className="row-actions" style={{marginTop: 'auto'}}>
                {ok && o.configPath && <>
                    <button onClick={() => api.openFolder('configs')}>打开配置文件夹</button>
                    {server.inLauncher
                        ? <button onClick={() => api.openLauncher()}>打开启动器</button>
                        : <button onClick={props.importToLauncher}>导入到启动器…</button>}
                </>}
                {!ok && o.errorCode === 'ssh_host_changed' && <button onClick={props.rekey}>重新读取指纹…</button>}
                {!ok && <button onClick={props.retry}>重试</button>}
                <button className="primary" style={{marginLeft: 'auto'}} onClick={props.done}>完成</button>
            </div>
        </Card>
    )
}
