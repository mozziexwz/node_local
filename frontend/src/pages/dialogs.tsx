import {useEffect, useState} from 'react'
import {api, errorText, itemKind, keyType, kindLabels, Probe, SecretKind, Server, TaskEvent, TaskKind} from '../api'
import {Banner, Dialog, Spinner} from '../components'

// ServerDialog adds a server (reading and trusting its host key) or edits
// one's remark, SSH port and login method.
export function ServerDialog(props: {server?: Server, onClose: () => void, onSaved: (s: Server) => void}) {
    const editing = props.server
    const [host, setHost] = useState(editing?.host ?? '')
    const [port, setPort] = useState(String(editing?.port ?? 22))
    const [name, setName] = useState(editing?.name ?? '')
    const [auth, setAuth] = useState<'password' | 'key'>(editing?.authMethod ?? 'password')
    const [keyPath, setKeyPath] = useState(editing?.keyPath ?? '')
    const [probe, setProbe] = useState<Probe | null>(null)
    const [busy, setBusy] = useState<'' | 'probe' | 'save'>('')
    const [error, setError] = useState('')

    // A host key read for one address says nothing about another.
    const changeAddress = (h: string, p: string) => {
        setHost(h)
        setPort(p)
        setProbe(null)
    }

    const read = async () => {
        setBusy('probe')
        setError('')
        try {
            setProbe(await api.probeHost(host.trim(), Number(port)))
        } catch (e) {
            setError(errorText(e))
        } finally {
            setBusy('')
        }
    }

    const pick = async () => {
        try {
            const path = await api.pickKeyFile()
            if (path) setKeyPath(path)
        } catch (e) {
            setError(errorText(e))
        }
    }

    const save = async () => {
        setBusy('save')
        setError('')
        const input = {
            name: name.trim(), host: host.trim(), port: Number(port), authMethod: auth, keyPath: auth === 'key' ? keyPath : '',
            fingerprint: probe?.fingerprint ?? editing?.fingerprint ?? '', algorithm: probe?.algorithm ?? editing?.algorithm ?? '',
        }
        try {
            props.onSaved(editing ? await api.updateServer(editing.id, input) : await api.addServer(input))
        } catch (e) {
            setError(errorText(e))
            setBusy('')
        }
    }

    const actions = editing ? <>
        <button onClick={props.onClose} disabled={busy !== ''}>取消</button>
        <button className="primary" onClick={save} disabled={busy !== ''}>{busy === 'save' && <Spinner/>}保存</button>
    </> : !probe ? <>
        <button onClick={props.onClose} disabled={busy !== ''}>取消</button>
        <button className="primary" onClick={read} disabled={busy !== '' || !host.trim()}>{busy === 'probe' && <Spinner/>}读取指纹</button>
    </> : <>
        <button className="left" onClick={read} disabled={busy !== ''}>{busy === 'probe' && <Spinner/>}重新读取</button>
        <button onClick={props.onClose} disabled={busy !== ''}>取消</button>
        <button className="primary" onClick={save} disabled={busy !== ''}>{busy === 'save' && <Spinner/>}信任并保存</button>
    </>

    return (
        <Dialog title={editing ? `编辑 ${editing.label}` : '添加服务器'} onClose={props.onClose} busy={busy !== ''} actions={actions}>
            <div className="form-grid">
                <label htmlFor="sv-host">服务器 IP</label>
                <input id="sv-host" value={host} disabled={!!editing} autoFocus={!editing} placeholder="VPS 的公网 IP"
                       onChange={e => changeAddress(e.target.value, port)}/>
                <label htmlFor="sv-port">SSH 端口</label>
                <div className="with">
                    <input id="sv-port" value={port} style={{width: 80}} inputMode="numeric"
                           onChange={e => changeAddress(host, e.target.value.replace(/\D/g, ''))}/>
                    <span className="hint">登录用户固定为 root</span>
                </div>
                <label htmlFor="sv-name">备注</label>
                <input id="sv-name" value={name} maxLength={30} placeholder="可不填，例如“东京节点”" onChange={e => setName(e.target.value)}/>
                <label>登录方式</label>
                <div className="with">
                    <span className="segmented">
                        <button className={auth === 'password' ? 'on' : ''} onClick={() => setAuth('password')}>密码</button>
                        <button className={auth === 'key' ? 'on' : ''} onClick={() => setAuth('key')}>私钥文件</button>
                    </span>
                    <span className="hint">{auth === 'password' ? '密码每次操作时再输入' : '只记文件位置，不复制私钥'}</span>
                </div>
                {auth === 'key' && <>
                    <label>私钥文件</label>
                    <div className="with">
                        <input className="grow" value={keyPath} readOnly placeholder="OpenSSH 格式的私钥文件"/>
                        <button onClick={pick}>选择…</button>
                    </div>
                </>}
            </div>
            {!editing && !probe && <p className="hint">先读取指纹：程序只和服务器握手，不登录，也不需要密码。</p>}
            {probe && (
                <div className="fp-box">
                    <span className="label">服务器指纹（{keyType(probe.algorithm)}）</span>
                    <span className="fp">{probe.fingerprint}</span>
                    <span className="small">首次连接无法排除中间人攻击。服务商控制台能看到 SSH 指纹的话，先核对一致。保存后指纹一旦变化，本程序会拒绝连接。</span>
                </div>
            )}
            {error && <Banner kind="error">{error}</Banner>}
        </Dialog>
    )
}

const startLabels: Partial<Record<TaskKind, string>> = {
    deploy: '开始部署', upgrade: '开始升级', fetch: '取回配置', status: '刷新状态', 'uninstall-preview': '读取卸载清单',
}

// SecretDialog asks for the root password or the key's passphrase for one
// operation and starts it.
export function SecretDialog(props: {server: Server, kind: TaskKind, need: SecretKind, onClose: () => void}) {
    const {server, kind, need} = props
    const [value, setValue] = useState('')
    const [show, setShow] = useState(false)
    const [busy, setBusy] = useState(false)
    const [error, setError] = useState('')
    const redeploy = kind === 'deploy' && !!server.node?.installed
    const what = need === 'password' ? 'root 密码' : '私钥口令'

    const submit = async () => {
        if (!value || busy) return
        setBusy(true)
        setError('')
        try {
            await api.startTask(kind, server.id, need === 'password' ? value : '', need === 'passphrase' ? value : '')
            props.onClose()
        } catch (e) {
            setError(errorText(e))
            setBusy(false)
        }
    }

    return (
        <Dialog title={`${redeploy ? '重新部署' : kindLabels[kind]}：输入${what}`} width={410} busy={busy} onClose={props.onClose} actions={<>
            <button onClick={props.onClose} disabled={busy}>取消</button>
            <button className={redeploy ? 'danger' : 'primary'} onClick={submit} disabled={busy || !value}>
                {busy && <Spinner/>}{redeploy ? '重新部署' : startLabels[kind]}
            </button>
        </>}>
            <p>连接 <b>{server.label}</b>（{server.host}:{server.port}），主机指纹已核对。</p>
            <div className="pw">
                <input type={show ? 'text' : 'password'} value={value} autoFocus aria-label={what}
                       onChange={e => setValue(e.target.value)} onKeyDown={e => e.key === 'Enter' && submit()}/>
                <button onClick={() => setShow(!show)}>{show ? '隐藏' : '显示'}</button>
            </div>
            {redeploy && <Banner kind="warn">这台服务器上已经有节点。重新部署会生成新配置，{server.configFile || '旧配置'} 等旧配置会立即失效。</Banner>}
            {error && <Banner kind="error">{error}</Banner>}
            <p className="hint">只用于这一次操作，用完即从内存清除，不写入硬盘或日志。</p>
        </Dialog>
    )
}

// FingerprintDialog reads the host key again and replaces the trusted one
// after the user has checked why it changed.
export function FingerprintDialog(props: {server: Server, onClose: () => void, onReplaced: () => void}) {
    const {server} = props
    const [probe, setProbe] = useState<Probe | null>(null)
    const [busy, setBusy] = useState(true)
    const [error, setError] = useState('')

    useEffect(() => {
        api.probeHost(server.host, server.port).then(setProbe).catch(e => setError(errorText(e))).finally(() => setBusy(false))
    }, [server])

    const same = probe?.fingerprint === server.fingerprint
    const replace = async () => {
        if (!probe) return
        setBusy(true)
        try {
            await api.replaceFingerprint(server.id, probe.fingerprint, probe.algorithm)
            props.onReplaced()
        } catch (e) {
            setError(errorText(e))
            setBusy(false)
        }
    }

    return (
        <Dialog title="重新读取服务器指纹" width={480} busy={busy} onClose={props.onClose} actions={<>
            <button onClick={props.onClose} disabled={busy}>取消</button>
            <button className="danger" onClick={replace} disabled={busy || !probe || same}>确认替换</button>
        </>}>
            <dl className="facts">
                <dt>原来的指纹</dt><dd className="fp">{server.fingerprint}</dd>
                <dt>现在的指纹</dt><dd className="fp">{busy && !probe ? <Spinner/> : probe ? `${probe.fingerprint}（${keyType(probe.algorithm)}）` : '—'}</dd>
            </dl>
            {same && <Banner kind="info">指纹没有变化，不需要替换。</Banner>}
            {probe && !same && (
                <Banner kind="warn">指纹变化通常是因为重装了系统或换了机器。如果你没有做过这些事，可能有人在冒充这台服务器，请不要替换，先在服务商控制台核实。</Banner>
            )}
            {error && <Banner kind="error">{error}</Banner>}
        </Dialog>
    )
}

// UninstallDialog shows what the preview found and confirms the removal.
export function UninstallDialog(props: {server: Server, task: TaskEvent, onClose: () => void}) {
    const items = props.task.outcome?.items ?? []
    const [busy, setBusy] = useState(false)
    const [error, setError] = useState('')

    const cancel = () => {
        api.discardUninstall(props.task.id)
        props.onClose()
    }
    const confirm = async () => {
        setBusy(true)
        try {
            await api.confirmUninstall(props.task.id)
        } catch (e) {
            setError(errorText(e))
            setBusy(false)
        }
    }

    return (
        <Dialog title={`卸载 ${props.server.label} 上的节点`} width={500} busy={busy} onClose={cancel} actions={<>
            <button autoFocus onClick={cancel} disabled={busy}>取消</button>
            <button className="danger" onClick={confirm} disabled={busy}>{busy && <Spinner/>}确认卸载</button>
        </>}>
            <p>将删除以下 {items.length} 项：</p>
            <ul className="paths">
                {items.map(i => <li key={i.kind + i.path}><span className="k">{itemKind(i.kind)}</span>{i.path}</li>)}
            </ul>
            <Banner kind="warn">卸载后，这台节点的所有配置文件都会失效。系统、其他软件和不是本程序加的防火墙规则不受影响。</Banner>
            {error && <Banner kind="error">{error}</Banner>}
        </Dialog>
    )
}
