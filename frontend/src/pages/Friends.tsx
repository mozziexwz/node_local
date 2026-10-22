import {useCallback, useEffect, useState} from 'react'
import {api, busyEvent, ConfigItem, endpoints, errorText, FriendCheck, hostOf, Overview, TaskEvent} from '../api'
import {Banner, Card, Spinner} from '../components'
import {useLaunch} from './launch'

type Message = {kind: 'error' | 'info', text: string}

const relaySteps = ['朋友复制节点地址发给你', '你在自己的服务器上建中转', '你把中转入口发回给朋友', '朋友用入口生成新配置']
const nodeSteps = ['你复制节点地址发给朋友', '朋友在他的服务器上建中转', '朋友把中转入口发回给你', '你用入口生成新配置']

export function Friends(props: {
    overview: Overview
    tasks: Record<string, TaskEvent>
    dismissTask: (serverId: string) => void
}) {
    const [role, setRole] = useState<'relay' | 'node'>('relay')
    const [message, setMessage] = useState<Message | null>(null)
    const showError = useCallback((text: string) => setMessage({kind: 'error', text}), [])
    const steps = role === 'relay' ? relaySteps : nodeSteps
    const mine = role === 'relay' ? [1, 2] : [0, 3]
    return (
        <div className="page fill">
            <div className="friend-top">
                <span className="segmented">
                    <button className={role === 'relay' ? 'on' : ''} onClick={() => setRole('relay')}>我帮朋友中转</button>
                    <button className={role === 'node' ? 'on' : ''} onClick={() => setRole('node')}>我用朋友的中转</button>
                </span>
                <span className="hint">双方只交换 IP 和端口，都不用交出 SSH 或节点密码</span>
            </div>
            <ol className="flow">
                {steps.map((s, i) => <li key={s} className={mine.includes(i) ? 'mine' : ''}><b>{i + 1}</b>{s}</li>)}
            </ol>
            {message && <Banner kind={message.kind} onClose={() => setMessage(null)}>{message.text}</Banner>}
            {role === 'relay'
                ? <RelayForFriend {...props} onError={showError}/>
                : <UseFriendRelay onMessage={setMessage}/>}
        </div>
    )
}

// RelayForFriend is the relay owner's side: steps 2 and 3.
function RelayForFriend(props: {
    overview: Overview
    tasks: Record<string, TaskEvent>
    dismissTask: (serverId: string) => void
    onError: (text: string) => void
}) {
    const list = props.overview.servers ?? []
    const [text, setText] = useState('')
    const [relayId, setRelayId] = useState(list[0]?.id ?? '')
    const [remark, setRemark] = useState('')
    const [rate, setRate] = useState('5')
    const [copied, setCopied] = useState(false)
    const {launch, dialog} = useLaunch(props.onError)
    const relay = list.find(s => s.id === relayId)
    const target = endpoints(text)[0] ?? ''
    const task = relay && props.tasks[relay.id]?.kind === 'forward' ? props.tasks[relay.id] : undefined
    const rateNumber = Number(rate)
    const ready = relay && target && Number.isInteger(rateNumber) && rateNumber >= 1 && rateNumber <= 10 && !busyEvent(props.tasks, relay.id)

    const create = () => {
        if (!relay || !ready) return
        props.dismissTask(relay.id)
        setCopied(false)
        launch(relay, 'forward', {target, remark: remark.trim(), rateMbps: rateNumber}, '创建中转')
    }
    const share = task?.state === 'succeeded' ? task.outcome?.share ?? '' : ''

    return <>
        <Card title={<><span className="num">2</span>建中转</>} actions={<span className="hint">建之前先从中转服务器连一下朋友的节点端口</span>}>
            <div className="form-grid wide">
                <label>朋友发来的</label>
                <div className="with">
                    <input className="grow" value={text} placeholder="粘贴「MSBOOST 中转请求 …」，或只填 IP:端口" onChange={e => setText(e.target.value)}/>
                    {text && (target ? <span className="ok small nowrap">✓ 目标 {target}</span> : <span className="bad small nowrap">没找到 IP:端口</span>)}
                </div>
                <label>中转服务器</label>
                <select value={relayId} onChange={e => setRelayId(e.target.value)}>
                    {list.length === 0 && <option value="">先在「服务器」页添加</option>}
                    {list.map(s => <option key={s.id} value={s.id}>{s.label}（{s.host}）</option>)}
                </select>
                <label>备注</label>
                <div className="with">
                    <input value={remark} maxLength={60} placeholder="朋友的名字" style={{width: 150}} onChange={e => setRemark(e.target.value)}/>
                    <span className="muted">带宽</span>
                    <input value={rate} style={{width: 52}} inputMode="numeric" onChange={e => setRate(e.target.value.replace(/\D/g, ''))}/>
                    <span className="muted">Mbps（1–10）</span>
                    <button className="primary" style={{marginLeft: 'auto'}} disabled={!ready} onClick={create}>创建中转</button>
                </div>
            </div>
        </Card>
        <Card title={<><span className="num">3</span>发给朋友</>} actions={task && task.state !== 'running' &&
            <span className={'small ' + (task.state === 'succeeded' ? 'ok' : 'bad')}>{task.state === 'succeeded' ? '中转已建好' : '没有建成'}</span>}>
            {!task && <p className="muted small">中转建好后，这里会出现一段发给朋友的文字。</p>}
            {task?.state === 'running' && <p className="muted"><Spinner/> 正在建中转，通常一分钟左右…</p>}
            {task && task.state !== 'running' && task.state !== 'succeeded' && (
                <Banner kind="error" onClose={() => props.dismissTask(task.serverId)}>
                    {task.outcome?.message}{task.outcome?.nextStep && <><br/>{task.outcome.nextStep}</>}
                </Banner>
            )}
            {share && <>
                <div className="share-out">
                    <span className="mono">{share}</span>
                    <button className="small primary" onClick={async () => {
                        await api.copy(share)
                        setCopied(true)
                    }}>{copied ? '已复制' : '复制'}</button>
                </div>
                {task?.outcome?.health?.publicTCP === 'unreachable' &&
                    <Banner kind="warn">这台电脑暂时连不上这个入口，请检查服务商安全组是否放行了入口端口。</Banner>}
                <p className="hint">朋友在本地版「我用朋友的中转」里粘贴这段文字就行。这条中转在「中转」页可以暂停、限速或删除；删除后朋友会连不上，删之前记得告诉他。</p>
            </>}
        </Card>
        <div className="notice">只帮信任的朋友建中转：朋友的游戏流量会经过你的服务器，占用你的带宽。中转只能指向公网地址，并且自带明文代理筛查。</div>
        {dialog}
    </>
}

// UseFriendRelay is the node owner's side: steps 1 and 4. Nothing logs in.
function UseFriendRelay(props: {onMessage: (m: Message | null) => void}) {
    const {onMessage} = props
    const [configs, setConfigs] = useState<ConfigItem[]>([])
    const [configName, setConfigName] = useState('')
    const [request, setRequest] = useState('')
    const [copied, setCopied] = useState(false)
    const [text, setText] = useState('')
    const [check, setCheck] = useState<FriendCheck | null>(null)
    const [checking, setChecking] = useState(false)
    const [checkError, setCheckError] = useState('')
    const [name, setName] = useState('')
    const [saving, setSaving] = useState(false)

    useEffect(() => {
        api.configs().then(c => {
            setConfigs(c)
            setConfigName(n => c.some(x => x.name === n) ? n : c[0]?.name ?? '')
        }).catch(e => onMessage({kind: 'error', text: errorText(e)}))
    }, [onMessage])

    useEffect(() => {
        setRequest('')
        setCopied(false)
        if (!configName) return
        api.forwardRequest(configName).then(setRequest).catch(e => onMessage({kind: 'error', text: errorText(e)}))
        setName(configName.replace(/\.json$/i, '') + '-经朋友中转')
    }, [configName, onMessage])

    // Check what the friend sent back once the typing stops.
    useEffect(() => {
        setCheck(null)
        setCheckError('')
        if (!configName || !text.trim()) return
        const id = window.setTimeout(() => {
            setChecking(true)
            api.checkFriendEntry(configName, text).then(setCheck).catch(e => setCheckError(errorText(e))).finally(() => setChecking(false))
        }, 400)
        return () => window.clearTimeout(id)
    }, [configName, text])

    const save = async () => {
        setSaving(true)
        onMessage(null)
        try {
            const path = await api.saveFriendConfig(configName, text, name.trim())
            onMessage({kind: 'info', text: `已保存为 ${path}。在启动器里选中它，「网络检测」的「当前到游戏服务器 IPv4」应该是你节点的 IP；原配置不变，随时能换回直连。`})
            setText('')
        } catch (e) {
            onMessage({kind: 'error', text: errorText(e)})
        } finally {
            setSaving(false)
        }
    }

    const nodeHost = check ? hostOf(check.node) : ''
    return <>
        <Card title={<><span className="num">1</span>把节点地址发给朋友</>} actions={<span className="hint">不需要登录任何服务器</span>}>
            <div className="form-grid wide">
                <label>我的节点配置</label>
                <div className="with">
                    <select value={configName} style={{width: 260}} onChange={e => setConfigName(e.target.value)}>
                        {configs.length === 0 && <option value="">配置文件夹里还没有节点配置</option>}
                        {configs.map(c => <option key={c.name} value={c.name}>{c.name}</option>)}
                    </select>
                    <span className="hint">网站或本地版部署的配置都可以</span>
                </div>
                <label>要发的内容</label>
                <div className="share-out">
                    <span className="mono">{request || '—'}</span>
                    <button className="small primary" disabled={!request} onClick={async () => {
                        await api.copy(request)
                        setCopied(true)
                    }}>{copied ? '已复制' : '复制'}</button>
                </div>
            </div>
            <p className="hint">只有节点的 IP 和端口，没有用户名和密码；朋友拿到它也用不了你的节点。</p>
        </Card>
        <Card title={<><span className="num">4</span>用朋友发回的入口生成配置</>}>
            <div className="form-grid wide">
                <label>朋友发回的</label>
                <input value={text} placeholder="粘贴「MSBOOST 中转入口 …」" onChange={e => setText(e.target.value)}/>
                <span/>
                <div className="checks">
                    {checking && <span className="muted"><Spinner/> 正在核对…</span>}
                    {checkError && <span className="bad">{checkError}</span>}
                    {check && <>
                        <span className="ok">✓ 入口 {check.entry}</span>
                        {check.matches === 'yes' && <span className="ok">✓ 中转目标是你这份配置的节点</span>}
                        {check.matches === 'no' && <span className="bad">✗ 中转目标是 {check.target}，不是你的节点 {nodeHost}</span>}
                        {check.matches === 'unknown' && <span className="warn-text">没写中转目标，生成后请用启动器的网络检测确认</span>}
                        {check.reachable === 'reachable' ? <span className="ok">✓ 本机到入口可达</span> : <span className="bad">✗ 本机连不上入口</span>}
                    </>}
                </div>
                <label>新配置名</label>
                <div className="with">
                    <input value={name} maxLength={60} style={{width: 240}} onChange={e => setName(e.target.value)}/>
                    <span className="hint">.json</span>
                    <button className="primary" style={{marginLeft: 'auto'}} disabled={!check || check.matches === 'no' || saving} onClick={save}>
                        {saving && <Spinner/>}生成配置
                    </button>
                </div>
            </div>
            <p className="hint">新配置存进启动器的配置文件夹，原配置不动。</p>
        </Card>
    </>
}
