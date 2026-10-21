import {useEffect, useState} from 'react'
import {About, api, errorText, Overview} from '../api'
import type {AskConfirm} from '../App'
import {Banner, Card, Modal, Spinner, Switch} from '../components'

export function Settings(props: {overview: Overview, refresh: () => Promise<void>, confirm: AskConfirm}) {
    const o = props.overview
    const [enabled, setEnabled] = useState(o.proxy.enabled)
    const [host, setHost] = useState(o.proxy.host)
    const [port, setPort] = useState(String(o.proxy.port))
    const [message, setMessage] = useState<{kind: 'error' | 'info', text: string} | null>(null)
    const [about, setAbout] = useState<About | null>(null)
    const [showLicenses, setShowLicenses] = useState(false)

    useEffect(() => {
        api.about().then(setAbout).catch(e => setMessage({kind: 'error', text: errorText(e)}))
    }, [])

    const changed = enabled !== o.proxy.enabled || host !== o.proxy.host || port !== String(o.proxy.port)
    const saveProxy = async () => {
        setMessage(null)
        try {
            await api.saveProxy(enabled, host, Number(port))
            await props.refresh()
            setMessage({kind: 'info', text: enabled ? `之后的 SSH 连接会经过 ${host}:${port}` : '之后的 SSH 连接直接连到服务器'})
        } catch (e) {
            setMessage({kind: 'error', text: errorText(e)})
        }
    }

    const run = async (fn: () => Promise<unknown>) => {
        setMessage(null)
        try {
            await fn()
            await props.refresh()
        } catch (e) {
            setMessage({kind: 'error', text: errorText(e)})
        }
    }

    const la = o.launcher
    return (
        <div className="page">
            {message && <Banner kind={message.kind} onClose={() => setMessage(null)}>{message.text}</Banner>}

            <Card title="SSH 经本机代理" actions={<Switch checked={enabled} onChange={setEnabled} label="SSH 经本机代理"/>}>
                <div className="row-actions">
                    <span className="muted">SOCKS5 地址</span>
                    <input value={host} disabled={!enabled} style={{width: 140}} onChange={e => setHost(e.target.value)}/>
                    <span className="muted">端口</span>
                    <input value={port} disabled={!enabled} style={{width: 80}} inputMode="numeric" onChange={e => setPort(e.target.value.replace(/\D/g, ''))}/>
                    <button className="primary" disabled={!changed} onClick={saveProxy}>保存</button>
                </div>
                <p className="hint">在国内直连 VPS 的 SSH 不稳定时打开，例如填 v2rayN 的本地 SOCKS 端口。部署后的端口检查仍然直连，测的是这台电脑到节点的真实情况。</p>
            </Card>

            <Card title="启动器与文件夹" actions={<>
                {la && !la.beside && <button className="small danger-text" onClick={() => run(api.forgetLauncher)}>忘记</button>}
                {!la?.beside && <button className="small" onClick={() => run(api.pickLauncher)}>{la ? '更改…' : '选择…'}</button>}
            </>}>
                <dl className="facts">
                    <dt>启动器</dt>
                    <dd>{la ? <><span className="path">{la.dir}</span><span className="hint">{la.beside ? '同一文件夹，自动识别' : '手动选择'}</span></>
                        : <span className="muted">还没找到。把本程序和 MSBOOST-Launcher.exe 放在同一文件夹，或者在这里选择启动器。</span>}</dd>
                    <dt>配置文件</dt>
                    <dd><span className="path">{o.configsDir}</span><button className="link" onClick={() => run(() => api.openFolder('configs'))}>打开</button></dd>
                    <dt>本程序数据</dt>
                    <dd><span className="path">{o.dataDir}</span><button className="link" onClick={() => run(() => api.openFolder('data'))}>打开</button>
                        <button className="link" onClick={() => run(() => api.openFolder('logs'))}>日志</button></dd>
                </dl>
                <p className="hint">部署和取回的配置存进配置文件夹{la ? '，启动器的「节点配置」页能直接看到' : ''}。本程序数据里有服务器列表、任务记录、设置和日志，都不含密码。</p>
            </Card>

            <Card title="关于" actions={about && <button className="small" onClick={() => setShowLicenses(true)}>开源许可…</button>}>
                {about ? <>
                    <dl className="facts">
                        <dt>版本</dt><dd>{about.version}{about.scripts && <span className="hint">{about.scripts}</span>}</dd>
                        <dt>版权</dt><dd>{about.copyright}</dd>
                    </dl>
                    <p className="notice">{about.license}</p>
                </> : <p className="muted"><Spinner/> 正在读取…</p>}
            </Card>

            {showLicenses && about && <Licenses about={about} onClose={() => setShowLicenses(false)}/>}
        </div>
    )
}

// Licenses lists the open-source components with their copyright notices.
function Licenses(props: {about: About, onClose: () => void}) {
    const rows = (list: About['components']) => list.map(c => (
        <tr key={c.name}>
            <td><button className="link" onClick={() => api.openURL(c.url)}>{c.name}</button></td>
            <td>{c.version || '—'}</td>
            <td>{c.copyright}</td>
            <td>{c.license}</td>
        </tr>
    ))
    return (
        <Modal title="开源许可" wide onClose={props.onClose}>
            <p>MSBOOST 本地版依据 GNU 通用公共许可证第 3 版（GPL-3.0）发布，使用了以下开源组件：</p>
            <table className="components">
                <thead><tr><th>组件</th><th>版本</th><th>版权</th><th>许可</th></tr></thead>
                <tbody>{rows(props.about.components)}</tbody>
            </table>
            <p className="license-gap">部署时由服务器从 GitHub 下载、安装在服务器上的组件（不包含在本程序里）：</p>
            <table className="components">
                <tbody>{rows(props.about.serverSide)}</tbody>
            </table>
            <div className="license-links">
                <span className="muted">许可全文：</span>
                {props.about.licenseTexts.map(l => (
                    <button key={l.name} className="link" onClick={() => api.openURL(l.url)}>{l.name}</button>
                ))}
            </div>
        </Modal>
    )
}
