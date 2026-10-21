// Typed access to the Go backend. The generated Wails bindings return class
// instances; they are used here as plain data with the shapes below.
import * as Go from '../wailsjs/go/main/App'
import {EventsOn} from '../wailsjs/runtime/runtime'

export type TaskKind = 'deploy' | 'upgrade' | 'fetch' | 'status' | 'uninstall-preview' | 'uninstall'
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
}

export interface TaskEvent {
    id: string
    serverId: string
    kind: TaskKind
    state: TaskState
    step: number // 1–10 while a deploy or upgrade runs
    started: number
    finished?: number
    outcome?: Outcome
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
    secretNeeded: async (id: string) => (await Go.SecretNeeded(id)) as SecretKind,
    startTask: async (kind: TaskKind, id: string, password: string, passphrase: string) =>
        plain<TaskEvent>(await Go.StartTask(kind, id, password, passphrase)),
    confirmUninstall: async (previewId: string) => plain<TaskEvent>(await Go.ConfirmUninstall(previewId)),
    discardUninstall: (previewId: string) => Go.DiscardUninstall(previewId),
    cancelTask: (id: string) => Go.CancelTask(id),
    records: async () => plain<TaskRecord[] | null>(await Go.Records()) ?? [],
    clearRecords: () => Go.ClearRecords(),
    saveProxy: (enabled: boolean, host: string, port: number) => Go.SaveProxy(enabled, host, port),
    pickLauncher: () => Go.PickLauncher(),
    forgetLauncher: () => Go.ForgetLauncher(),
    copyToLauncher: (id: string) => Go.CopyToLauncher(id),
    openFolder: (kind: 'configs' | 'data' | 'logs') => Go.OpenFolder(kind),
    openLauncher: () => Go.OpenLauncher(),
    about: async () => plain<About>(await Go.About()),
    openURL: (url: string) => Go.OpenURL(url),
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
}

// What a running task shows in the server list.
export const busyLabels: Record<TaskKind, string> = {
    deploy: '部署中',
    upgrade: '升级中',
    fetch: '取回中',
    status: '检查中',
    'uninstall-preview': '读取中',
    uninstall: '卸载中',
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
