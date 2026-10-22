import {useEffect, useState} from 'react'
import {api, errorText, kindLabels, Overview, TaskRecord} from '../api'
import type {AskConfirm} from '../App'
import {Banner, Card, formatElapsed, formatTime} from '../components'

const stateLabels: Record<string, string> = {succeeded: '成功', failed: '未完成', cancelled: '已取消'}

export function Records(props: {overview: Overview, confirm: AskConfirm}) {
    const [records, setRecords] = useState<TaskRecord[] | null>(null)
    const [selected, setSelected] = useState('')
    const [error, setError] = useState('')

    // The overview changes whenever a task finishes.
    useEffect(() => {
        api.records().then(list => {
            setRecords(list)
            setSelected(s => list.some(r => r.id === s) ? s : list[0]?.id ?? '')
        }).catch(e => setError(errorText(e)))
    }, [props.overview])

    const clear = async () => {
        if (!await props.confirm('清空任务记录', '删除全部任务记录？服务器和配置文件都不受影响。', '清空', true)) return
        try {
            await api.clearRecords()
            setRecords([])
        } catch (e) {
            setError(errorText(e))
        }
    }

    const record = records?.find(r => r.id === selected)
    const o = record?.outcome
    return (
        <div className="page fill">
            {error && <Banner kind="error" onClose={() => setError('')}>{error}</Banner>}
            <Card title="任务记录" className="records" actions={records && records.length > 0 && <button className="small" onClick={clear}>清空记录</button>}>
                {records && records.length === 0 && <p className="empty">还没有任务记录。部署、升级、取回配置、建中转和卸载会记在这里。</p>}
                {records && records.length > 0 && (
                    <div className="table-scroll">
                        <table className="list">
                            <thead><tr><th>时间</th><th>服务器</th><th>操作</th><th>结果</th><th>用时</th></tr></thead>
                            <tbody>
                                {records.map(r => (
                                    <tr key={r.id} className={r.id === selected ? 'sel' : ''} onClick={() => setSelected(r.id)}>
                                        <td>{formatTime(r.started)}</td>
                                        <td>{r.server}</td>
                                        <td>{kindLabels[r.kind]}</td>
                                        <td className={r.state === 'succeeded' ? 'ok' : 'bad'}>
                                            {r.state === 'succeeded' ? '成功' : r.outcome.message || stateLabels[r.state]}
                                        </td>
                                        <td>{formatElapsed(r.finished - r.started)}</td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                )}
            </Card>
            {record && o && (
                <Card title={`${formatTime(record.started)} ${kindLabels[record.kind]} · ${record.server}`}
                      actions={<span className={'small ' + (record.state === 'succeeded' ? 'ok' : 'bad')}>{stateLabels[record.state]}</span>}>
                    <dl className="facts">
                        <dt>服务器</dt><dd>{record.address}，指纹 <span className="fp">{record.fingerprint}</span></dd>
                        {o.message && <><dt>结果</dt><dd>{o.message}</dd></>}
                        {o.nextStep && <><dt>建议</dt><dd>{o.nextStep}</dd></>}
                        {o.errorCode && <><dt>诊断编号</dt><dd>{o.errorCode}</dd></>}
                        {o.share ? <><dt>发给朋友</dt><dd className="path">{o.share}</dd></>
                            : o.entry && <><dt>中转入口</dt><dd className="path">{o.entry}</dd></>}
                        {o.configPath && <><dt>配置文件</dt><dd className="path">{o.configPath}</dd></>}
                    </dl>
                </Card>
            )}
            <p className="hint">记录里没有密码，也没有配置文件的内容。</p>
        </div>
    )
}
