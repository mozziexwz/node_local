// Typed access to the Go backend. The generated Wails bindings return class
// instances; they are used here as plain data with the shapes below.
import * as Go from '../wailsjs/go/main/App'
import {ClipboardSetText, EventsOn} from '../wailsjs/runtime/runtime'

export type TaskKind =
    'deploy' | 'upgrade' | 'fetch' | 'status' | 'uninstall-preview' | 'uninstall' |
    'relay' | 'forward' | 'relay-list' | 'relay-manage' | 'relay-uninstall-preview' | 'relay-uninstall'
export type TaskState = 'running' | 'succeeded' | 'failed' | 'cancelled'

export interface Health {
    bbr?: string
    service: string
    localSelfTest: string
    publicTCP: string
    game: string
}

export interface CleanupItem {
    path: string
    kind: string // file, directory or firewall
}

export interface NodeState {
    installed: boolean
    service?: string // running or stopped
    engine?: string
    bbr?: string
    os?: string
    entry?: string
    publicTCP?: string
    checkedAt: number
}

export interface Hop {
    fromHost: string
    fromPort: number
    toHost: string
    toPort: number
}

export interface RelayRow {
    id: string
    role: string // relay, front or route
    port: number
    entry: string
    rateMbps: number
    target: string
    state: string // running, paused or stopped
    connections: number
    createdAt: number
    source: string // self, friend, front, website, code or unknown
    remark: string
}

export interface Managed {
    id: string
    ok: boolean
    error?: string
}

export interface Outcome {
    phase?: string
    errorCode?: string
    message?: string
    nextStep?: string
    health?: Health
    configFile?: string
    configPath?: string
    items?: CleanupItem[]
    node?: NodeState
    hops?: Hop[]
    entry?: string
    share?: string
    relays?: RelayRow[]
    managed?: Managed[]
}

export interface TaskEvent {
    id: string
    serverId: string
    frontId?: string
    kind: TaskKind
    state: TaskState
    step: number // 1–10 while a deploy or upgrade runs
    started: number
    finished?: number
    outcome?: Outcome
}

export interface RelaySummary {
    count: number
    checkedAt: number
}

export interface Server {
    id: string
    label: string
    name: string
    host: string
    port: number
    authMethod: 'password' | 'key'
    keyPath: string
    fingerprint: string
    algorithm: string
    trustedAt: number
    configFile: string
    configPath: string // "" when the file is gone
    inLauncher: boolean
    node: NodeState | null
    relays: RelaySummary | null
}

export interface Proxy {
    enabled: boolean
    host: string
    port: number
}

export interface Launcher {
    dir: string
    beside: boolean
}

export interface Overview {
    servers: Server[] | null
    running: TaskEvent[] | null
    proxy: Proxy
    launcher: Launcher | null
    configsDir: string
    dataDir: string
    version: string
    warnings: string[] | null
}

export interface TaskRecord {
    id: string
    serverId: string
    server: string
    address: string
    fingerprint: string
    kind: TaskKind
    state: TaskState
    started: number
    finished: number
    outcome: Outcome
}

export interface ServerInput {
    name: string
    host: string
    port: number
    authMethod: 'password' | 'key'
    keyPath: string
    fingerprint: string
    algorithm: string
}

export interface TaskOptions {
    configName?: string
    frontId?: string
    frontPassword?: string
    frontPassphrase?: string
    remark?: string
    rateMbps?: number
    target?: string
    action?: string
    items?: string[]
}

export interface ConfigItem {
    name: string
    entry: string
}

export interface FriendCheck {
    entry: string
    target: string
    node: string
    matches: 'yes' | 'no' | 'unknown'
    reachable: 'reachable' | 'unreachable'
}

export interface Probe {
    fingerprint: string
    algorithm: string
}

export interface Component {
    name: string
    version: string
    copyright: string
    license: string
    url: string
}

export interface Link {
    name: string
    url: string
}

export interface About {
    version: string
    copyright: string
    license: string
    source: string // where a release's source is offered; '' for test builds
    scripts: string
    components: Component[]
    serverSide: Component[]
    licenseTexts: Link[]
}

export type SecretKind = '' | 'password' | 'passphrase'

const plain = <T>(v: unknown) => v as T

export const api = {
    overview: async () => plain<Overview>(await Go.Overview()),
    probeHost: async (host: string, port: number) => plain<Probe>(await Go.ProbeHost(host, port)),
    addServer: async (s: ServerInput) => plain<Server>(await Go.AddServer(s as never)),
    updateServer: async (id: string, s: ServerInput) => plain<Server>(await Go.UpdateServer(id, s as never)),
    replaceFingerprint: async (id: string, fingerprint: string, algorithm: string) =>
        plain<Server>(await Go.ReplaceFingerprint(id, fingerprint, algorithm)),
    removeServer: (id: string) => Go.RemoveServer(id),
    pickKeyFile: () => Go.PickKeyFile(),
    secretNeeded: async (id: string, kind: TaskKind) => (await Go.SecretNeeded(id, kind)) as SecretKind,
    startTask: async (kind: TaskKind, id: string, password: string, passphrase: string, opts: TaskOptions = {}) =>
        plain<TaskEvent>(await Go.StartTask(kind, id, password, passphrase, opts as never)),
    confirmUninstall: async (previewId: string) => plain<TaskEvent>(await Go.ConfirmUninstall(previewId)),
    discardUninstall: (previewId: string) => Go.DiscardUninstall(previewId),
    cancelTask: (id: string) => Go.CancelTask(id),
    records: async () => plain<TaskRecord[] | null>(await Go.Records()) ?? [],
    clearRecords: () => Go.ClearRecords(),
    configs: async () => plain<ConfigItem[] | null>(await Go.Configs()) ?? [],
    forwardRequest: (configName: string) => Go.ForwardRequest(configName),
    checkFriendEntry: async (configName: string, text: string) => plain<FriendCheck>(await Go.CheckFriendEntry(configName, text)),
    saveFriendConfig: (configName: string, text: string, name: string) => Go.SaveFriendConfig(configName, text, name),
    saveProxy: (enabled: boolean, host: string, port: number) => Go.SaveProxy(enabled, host, port),
    pickLauncher: () => Go.PickLauncher(),
    forgetLauncher: () => Go.ForgetLauncher(),
    copyToLauncher: (id: string) => Go.CopyToLauncher(id),
    openFolder: (kind: 'configs' | 'data' | 'logs') => Go.OpenFolder(kind),
    openLauncher: () => Go.OpenLauncher(),
    about: async () => plain<About>(await Go.About()),
    openURL: (url: string) => Go.OpenURL(url),
    copy: (text: string) => ClipboardSetText(text),
}

export const on = {
    task: (fn: (e: TaskEvent) => void) => EventsOn('task', fn),
}

// errorText turns a rejected binding call into a readable message.
export function errorText(err: unknown): string {
    if (typeof err === 'string') return err
    if (err instanceof Error) return err.message
    return String(err)
}

// The steps of a deploy or upgrade, numbered like executor.Step.
export const stepLabels = [
    '连接服务器，核对指纹',
    '检查系统',
    '开启 BBR',
    '安装依赖软件',
    '同步系统时间',
    '下载并校验 MSBOOST 引擎',
    '下载分流规则',
    '检查配置并安装服务',
    '本机自测',
    '保存配置文件',
]

export const kindLabels: Record<TaskKind, string> = {
    deploy: '部署节点',
    upgrade: '升级节点',
    fetch: '取回配置',
    status: '刷新状态',
    'uninstall-preview': '读取卸载清单',
    uninstall: '卸载节点',
    relay: '新建自备中转',
    forward: '帮朋友建中转',
    'relay-list': '读取中转列表',
    'relay-manage': '管理中转',
    'relay-uninstall-preview': '读取中转卸载清单',
    'relay-uninstall': '卸载全部中转',
}

// What a running task shows in the server list.
export const busyLabels: Record<TaskKind, string> = {
    deploy: '部署中',
    upgrade: '升级中',
    fetch: '取回中',
    status: '检查中',
    'uninstall-preview': '读取中',
    uninstall: '卸载中',
    relay: '建中转中',
    forward: '建中转中',
    'relay-list': '读取中',
    'relay-manage': '处理中',
    'relay-uninstall-preview': '读取中',
    'relay-uninstall': '卸载中',
}

// itemKind names an uninstall item's kind.
export function itemKind(kind: string): string {
    return ({file: '文件', directory: '目录', firewall: '防火墙'} as Record<string, string>)[kind] ?? kind
}

// keyType shortens an SSH host key algorithm for display.
export function keyType(algorithm: string): string {
    if (algorithm === 'ssh-ed25519') return 'ED25519'
    if (algorithm.startsWith('ecdsa')) return 'ECDSA'
    if (algorithm === 'ssh-rsa' || algorithm.startsWith('rsa')) return 'RSA'
    return algorithm
}

// busyEvent is the running task that uses a server, as its target or front.
export function busyEvent(tasks: Record<string, TaskEvent>, serverId: string): TaskEvent | undefined {
    const own = tasks[serverId]
    if (own?.state === 'running') return own
    return Object.values(tasks).find(t => t.state === 'running' && t.frontId === serverId)
}

// endpoints finds IP:port pairs in pasted text, like the Go side.
export function endpoints(text: string): string[] {
    return text.match(/\[[0-9A-Fa-f:.]+\]:\d{1,5}|(?:\d{1,3}\.){3}\d{1,3}:\d{1,5}/g) ?? []
}

// hostOf is the host of "ip:port" or "[ipv6]:port".
export function hostOf(entry: string): string {
    return entry.replace(/:\d+$/, '').replace(/^\[(.*)\]$/, '$1')
}

// sameHost compares two hosts as written. The Go side checks again and also
// catches other spellings of one IPv6 address.
export function sameHost(a: string, b: string): boolean {
    return a !== '' && a.toLowerCase() === b.toLowerCase()
}
