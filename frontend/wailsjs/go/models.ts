export namespace executor {
	
	export class CleanupItem {
	    path: string;
	    kind: string;
	
	    static createFrom(source: any = {}) {
	        return new CleanupItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.kind = source["kind"];
	    }
	}
	export class Health {
	    bbr?: string;
	    service: string;
	    localSelfTest: string;
	    publicTCP: string;
	    game: string;
	
	    static createFrom(source: any = {}) {
	        return new Health(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.bbr = source["bbr"];
	        this.service = source["service"];
	        this.localSelfTest = source["localSelfTest"];
	        this.publicTCP = source["publicTCP"];
	        this.game = source["game"];
	    }
	}
	export class Hop {
	    fromHost: string;
	    fromPort: number;
	    toHost: string;
	    toPort: number;
	
	    static createFrom(source: any = {}) {
	        return new Hop(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fromHost = source["fromHost"];
	        this.fromPort = source["fromPort"];
	        this.toHost = source["toHost"];
	        this.toPort = source["toPort"];
	    }
	}
	export class RelayManageResult {
	    id: string;
	    ok: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new RelayManageResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.ok = source["ok"];
	        this.error = source["error"];
	    }
	}

}

export namespace main {
	
	export class LinkDTO {
	    name: string;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new LinkDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.url = source["url"];
	    }
	}
	export class ComponentDTO {
	    name: string;
	    version: string;
	    copyright: string;
	    license: string;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new ComponentDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.copyright = source["copyright"];
	        this.license = source["license"];
	        this.url = source["url"];
	    }
	}
	export class AboutDTO {
	    version: string;
	    copyright: string;
	    license: string;
	    scripts: string;
	    components: ComponentDTO[];
	    serverSide: ComponentDTO[];
	    licenseTexts: LinkDTO[];
	
	    static createFrom(source: any = {}) {
	        return new AboutDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.copyright = source["copyright"];
	        this.license = source["license"];
	        this.scripts = source["scripts"];
	        this.components = this.convertValues(source["components"], ComponentDTO);
	        this.serverSide = this.convertValues(source["serverSide"], ComponentDTO);
	        this.licenseTexts = this.convertValues(source["licenseTexts"], LinkDTO);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ConfigDTO {
	    name: string;
	    entry: string;
	
	    static createFrom(source: any = {}) {
	        return new ConfigDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.entry = source["entry"];
	    }
	}
	export class FriendCheckDTO {
	    entry: string;
	    target: string;
	    node: string;
	    matches: string;
	    reachable: string;
	
	    static createFrom(source: any = {}) {
	        return new FriendCheckDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.entry = source["entry"];
	        this.target = source["target"];
	        this.node = source["node"];
	        this.matches = source["matches"];
	        this.reachable = source["reachable"];
	    }
	}
	
	export class ServerDTO {
	    id: string;
	    label: string;
	    name: string;
	    host: string;
	    port: number;
	    authMethod: string;
	    keyPath: string;
	    fingerprint: string;
	    algorithm: string;
	    trustedAt: number;
	    configFile: string;
	    configPath: string;
	    inLauncher: boolean;
	    node?: servers.NodeState;
	    relays?: servers.RelaySummary;
	
	    static createFrom(source: any = {}) {
	        return new ServerDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.authMethod = source["authMethod"];
	        this.keyPath = source["keyPath"];
	        this.fingerprint = source["fingerprint"];
	        this.algorithm = source["algorithm"];
	        this.trustedAt = source["trustedAt"];
	        this.configFile = source["configFile"];
	        this.configPath = source["configPath"];
	        this.inLauncher = source["inLauncher"];
	        this.node = this.convertValues(source["node"], servers.NodeState);
	        this.relays = this.convertValues(source["relays"], servers.RelaySummary);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class OverviewDTO {
	    servers: ServerDTO[];
	    running: tasks.Event[];
	    proxy: store.Proxy;
	    launcher?: store.Launcher;
	    configsDir: string;
	    dataDir: string;
	    version: string;
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new OverviewDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.servers = this.convertValues(source["servers"], ServerDTO);
	        this.running = this.convertValues(source["running"], tasks.Event);
	        this.proxy = this.convertValues(source["proxy"], store.Proxy);
	        this.launcher = this.convertValues(source["launcher"], store.Launcher);
	        this.configsDir = source["configsDir"];
	        this.dataDir = source["dataDir"];
	        this.version = source["version"];
	        this.warnings = source["warnings"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ProbeDTO {
	    fingerprint: string;
	    algorithm: string;
	
	    static createFrom(source: any = {}) {
	        return new ProbeDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fingerprint = source["fingerprint"];
	        this.algorithm = source["algorithm"];
	    }
	}
	
	export class ServerInput {
	    name: string;
	    host: string;
	    port: number;
	    authMethod: string;
	    keyPath: string;
	    fingerprint: string;
	    algorithm: string;
	
	    static createFrom(source: any = {}) {
	        return new ServerInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.authMethod = source["authMethod"];
	        this.keyPath = source["keyPath"];
	        this.fingerprint = source["fingerprint"];
	        this.algorithm = source["algorithm"];
	    }
	}
	export class TaskOptions {
	    configName: string;
	    frontId: string;
	    frontPassword: string;
	    frontPassphrase: string;
	    remark: string;
	    rateMbps: number;
	    target: string;
	    action: string;
	    items: string[];
	
	    static createFrom(source: any = {}) {
	        return new TaskOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.configName = source["configName"];
	        this.frontId = source["frontId"];
	        this.frontPassword = source["frontPassword"];
	        this.frontPassphrase = source["frontPassphrase"];
	        this.remark = source["remark"];
	        this.rateMbps = source["rateMbps"];
	        this.target = source["target"];
	        this.action = source["action"];
	        this.items = source["items"];
	    }
	}

}

export namespace servers {
	
	export class NodeState {
	    installed: boolean;
	    service?: string;
	    engine?: string;
	    bbr?: string;
	    os?: string;
	    entry?: string;
	    publicTCP?: string;
	    checkedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new NodeState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.installed = source["installed"];
	        this.service = source["service"];
	        this.engine = source["engine"];
	        this.bbr = source["bbr"];
	        this.os = source["os"];
	        this.entry = source["entry"];
	        this.publicTCP = source["publicTCP"];
	        this.checkedAt = source["checkedAt"];
	    }
	}
	export class RelaySummary {
	    count: number;
	    checkedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new RelaySummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.count = source["count"];
	        this.checkedAt = source["checkedAt"];
	    }
	}

}

export namespace store {
	
	export class Launcher {
	    dir: string;
	    beside: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Launcher(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dir = source["dir"];
	        this.beside = source["beside"];
	    }
	}
	export class Proxy {
	    enabled: boolean;
	    host: string;
	    port: number;
	
	    static createFrom(source: any = {}) {
	        return new Proxy(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.host = source["host"];
	        this.port = source["port"];
	    }
	}

}

export namespace tasks {
	
	export class RelayRow {
	    id: string;
	    role: string;
	    port: number;
	    entry: string;
	    rateMbps: number;
	    target: string;
	    state: string;
	    connections: number;
	    createdAt: number;
	    source: string;
	    remark: string;
	
	    static createFrom(source: any = {}) {
	        return new RelayRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.role = source["role"];
	        this.port = source["port"];
	        this.entry = source["entry"];
	        this.rateMbps = source["rateMbps"];
	        this.target = source["target"];
	        this.state = source["state"];
	        this.connections = source["connections"];
	        this.createdAt = source["createdAt"];
	        this.source = source["source"];
	        this.remark = source["remark"];
	    }
	}
	export class Outcome {
	    phase?: string;
	    errorCode?: string;
	    message?: string;
	    nextStep?: string;
	    health?: executor.Health;
	    configFile?: string;
	    configPath?: string;
	    items?: executor.CleanupItem[];
	    node?: servers.NodeState;
	    hops?: executor.Hop[];
	    entry?: string;
	    share?: string;
	    relays?: RelayRow[];
	    managed?: executor.RelayManageResult[];
	
	    static createFrom(source: any = {}) {
	        return new Outcome(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.phase = source["phase"];
	        this.errorCode = source["errorCode"];
	        this.message = source["message"];
	        this.nextStep = source["nextStep"];
	        this.health = this.convertValues(source["health"], executor.Health);
	        this.configFile = source["configFile"];
	        this.configPath = source["configPath"];
	        this.items = this.convertValues(source["items"], executor.CleanupItem);
	        this.node = this.convertValues(source["node"], servers.NodeState);
	        this.hops = this.convertValues(source["hops"], executor.Hop);
	        this.entry = source["entry"];
	        this.share = source["share"];
	        this.relays = this.convertValues(source["relays"], RelayRow);
	        this.managed = this.convertValues(source["managed"], executor.RelayManageResult);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Event {
	    id: string;
	    serverId: string;
	    frontId?: string;
	    kind: string;
	    state: string;
	    step: number;
	    started: number;
	    finished?: number;
	    outcome?: Outcome;
	
	    static createFrom(source: any = {}) {
	        return new Event(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.serverId = source["serverId"];
	        this.frontId = source["frontId"];
	        this.kind = source["kind"];
	        this.state = source["state"];
	        this.step = source["step"];
	        this.started = source["started"];
	        this.finished = source["finished"];
	        this.outcome = this.convertValues(source["outcome"], Outcome);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class Record {
	    id: string;
	    serverId: string;
	    server: string;
	    address: string;
	    fingerprint: string;
	    kind: string;
	    state: string;
	    started: number;
	    finished: number;
	    outcome: Outcome;
	
	    static createFrom(source: any = {}) {
	        return new Record(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.serverId = source["serverId"];
	        this.server = source["server"];
	        this.address = source["address"];
	        this.fingerprint = source["fingerprint"];
	        this.kind = source["kind"];
	        this.state = source["state"];
	        this.started = source["started"];
	        this.finished = source["finished"];
	        this.outcome = this.convertValues(source["outcome"], Outcome);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

