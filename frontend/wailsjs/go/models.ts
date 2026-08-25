export namespace config {
	
	export class Subscription {
	    id: string;
	    name: string;
	    url: string;
	    updatedAt?: string;
	    trafficUpload?: number;
	    trafficDownload?: number;
	    trafficTotal?: number;
	    expireUnix?: number;
	    iconUrl?: string;
	    source?: string;
	    allowInsecure?: boolean;
	    removedRoutingListUrls?: string[];
	
	    static createFrom(source: any = {}) {
	        return new Subscription(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.url = source["url"];
	        this.updatedAt = source["updatedAt"];
	        this.trafficUpload = source["trafficUpload"];
	        this.trafficDownload = source["trafficDownload"];
	        this.trafficTotal = source["trafficTotal"];
	        this.expireUnix = source["expireUnix"];
	        this.iconUrl = source["iconUrl"];
	        this.source = source["source"];
	        this.allowInsecure = source["allowInsecure"];
	        this.removedRoutingListUrls = source["removedRoutingListUrls"];
	    }
	}
	export class AppSettings {
	    autostart: boolean;
	    killswitch: boolean;
	    mode: string;
	    language: string;
	    theme: string;
	    lastSelectedProxyId?: string;
	    localPort?: number;
	    listenLan?: boolean;
	    dnsServers?: string[];
	    tunIpv4?: string;
	    tunStack?: string;
	    favorites?: string[];
	    subscriptionAutoUpdate?: boolean;
	    subscriptionUpdateIntervalHours?: number;
	    subscriptionSendHWID?: boolean;
	    subscriptionUserAgent?: string;
	    dnsLeakProtection?: boolean;
	    enableIPv6?: boolean;
	    routingListUpdateHours?: number;
	    lastChangelogVersion?: string;
	
	    static createFrom(source: any = {}) {
	        return new AppSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.autostart = source["autostart"];
	        this.killswitch = source["killswitch"];
	        this.mode = source["mode"];
	        this.language = source["language"];
	        this.theme = source["theme"];
	        this.lastSelectedProxyId = source["lastSelectedProxyId"];
	        this.localPort = source["localPort"];
	        this.listenLan = source["listenLan"];
	        this.dnsServers = source["dnsServers"];
	        this.tunIpv4 = source["tunIpv4"];
	        this.tunStack = source["tunStack"];
	        this.favorites = source["favorites"];
	        this.subscriptionAutoUpdate = source["subscriptionAutoUpdate"];
	        this.subscriptionUpdateIntervalHours = source["subscriptionUpdateIntervalHours"];
	        this.subscriptionSendHWID = source["subscriptionSendHWID"];
	        this.subscriptionUserAgent = source["subscriptionUserAgent"];
	        this.dnsLeakProtection = source["dnsLeakProtection"];
	        this.enableIPv6 = source["enableIPv6"];
	        this.routingListUpdateHours = source["routingListUpdateHours"];
	        this.lastChangelogVersion = source["lastChangelogVersion"];
	    }
	}
	export class ProxyEntry {
	    id: string;
	    ip: string;
	    port: number;
	    type: string;
	    username: string;
	    password: string;
	    name: string;
	    country: string;
	    uri?: string;
	    extra?: number[];
	    provider?: string;
	    subscriptionUrl?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProxyEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.ip = source["ip"];
	        this.port = source["port"];
	        this.type = source["type"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.name = source["name"];
	        this.country = source["country"];
	        this.uri = source["uri"];
	        this.extra = source["extra"];
	        this.provider = source["provider"];
	        this.subscriptionUrl = source["subscriptionUrl"];
	    }
	}
	export class RoutingList {
	    id: string;
	    name: string;
	    url: string;
	    action: string;
	    enabled: boolean;
	    allowInsecure?: boolean;
	    updatedAt?: number;
	    domainCount?: number;
	    cidrCount?: number;
	    lastError?: string;
	    subscriptionId?: string;
	
	    static createFrom(source: any = {}) {
	        return new RoutingList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.url = source["url"];
	        this.action = source["action"];
	        this.enabled = source["enabled"];
	        this.allowInsecure = source["allowInsecure"];
	        this.updatedAt = source["updatedAt"];
	        this.domainCount = source["domainCount"];
	        this.cidrCount = source["cidrCount"];
	        this.lastError = source["lastError"];
	        this.subscriptionId = source["subscriptionId"];
	    }
	}
	export class RoutingRules {
	    mode: string;
	    whitelist: string[];
	    appWhitelist: string[];
	    appForceVPN: string[];
	    customBlockedDomains: string[];
	    routingLists: RoutingList[];
	
	    static createFrom(source: any = {}) {
	        return new RoutingRules(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.whitelist = source["whitelist"];
	        this.appWhitelist = source["appWhitelist"];
	        this.appForceVPN = source["appForceVPN"];
	        this.customBlockedDomains = source["customBlockedDomains"];
	        this.routingLists = this.convertValues(source["routingLists"], RoutingList);
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
	export class AppConfig {
	    routingRules: RoutingRules;
	    proxies: ProxyEntry[];
	    settings: AppSettings;
	    subscriptions?: Subscription[];
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.routingRules = this.convertValues(source["routingRules"], RoutingRules);
	        this.proxies = this.convertValues(source["proxies"], ProxyEntry);
	        this.settings = this.convertValues(source["settings"], AppSettings);
	        this.subscriptions = this.convertValues(source["subscriptions"], Subscription);
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

export namespace logger {
	
	export class LogEntry {
	    timestamp: number;
	    time: string;
	    msg: string;
	    type: string;
	    source: string;
	    icon: string;
	    domain: string;
	
	    static createFrom(source: any = {}) {
	        return new LogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timestamp = source["timestamp"];
	        this.time = source["time"];
	        this.msg = source["msg"];
	        this.type = source["type"];
	        this.source = source["source"];
	        this.icon = source["icon"];
	        this.domain = source["domain"];
	    }
	}
	export class LogPage {
	    items: LogEntry[];
	    total: number;
	    page: number;
	    pageSize: number;
	    totalPages: number;
	
	    static createFrom(source: any = {}) {
	        return new LogPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], LogEntry);
	        this.total = source["total"];
	        this.page = source["page"];
	        this.pageSize = source["pageSize"];
	        this.totalPages = source["totalPages"];
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

export namespace main {
	
	export class AutoGroupStatus {
	    nodeName: string;
	    nodeIp: string;
	    rttMs: number;
	    known: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AutoGroupStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.nodeName = source["nodeName"];
	        this.nodeIp = source["nodeIp"];
	        this.rttMs = source["rttMs"];
	        this.known = source["known"];
	    }
	}
	export class ChangelogItem {
	    type: string;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new ChangelogItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.text = source["text"];
	    }
	}
	export class Changelog {
	    version: string;
	    title: string;
	    items: ChangelogItem[];
	
	    static createFrom(source: any = {}) {
	        return new Changelog(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.title = source["title"];
	        this.items = this.convertValues(source["items"], ChangelogItem);
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
	
	export class SubscriptionPreview {
	    proxies: config.ProxyEntry[];
	    routingLists: config.RoutingList[];
	
	    static createFrom(source: any = {}) {
	        return new SubscriptionPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.proxies = this.convertValues(source["proxies"], config.ProxyEntry);
	        this.routingLists = this.convertValues(source["routingLists"], config.RoutingList);
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

export namespace proxy {
	
	export class ConnectResultDTO {
	    success: boolean;
	    message: string;
	    gpoConflict: boolean;
	    tunnelFailed: boolean;
	    reason: string;
	    fallbackUsed: boolean;
	    errorCode?: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectResultDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.message = source["message"];
	        this.gpoConflict = source["gpoConflict"];
	        this.tunnelFailed = source["tunnelFailed"];
	        this.reason = source["reason"];
	        this.fallbackUsed = source["fallbackUsed"];
	        this.errorCode = source["errorCode"];
	    }
	}
	export class PingResultDTO {
	    reachable: boolean;
	    latencyMs: number;
	    reason?: string;
	    checkType?: string;
	
	    static createFrom(source: any = {}) {
	        return new PingResultDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.reachable = source["reachable"];
	        this.latencyMs = source["latencyMs"];
	        this.reason = source["reason"];
	        this.checkType = source["checkType"];
	    }
	}
	export class ProxyConfig {
	    id?: string;
	    ip: string;
	    port: number;
	    type: string;
	    username: string;
	    password: string;
	    uri?: string;
	    extra?: number[];
	    subscriptionUrl?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProxyConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.ip = source["ip"];
	        this.port = source["port"];
	        this.type = source["type"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.uri = source["uri"];
	        this.extra = source["extra"];
	        this.subscriptionUrl = source["subscriptionUrl"];
	    }
	}
	export class StatusDTO {
	    isConnected: boolean;
	    isEstablishing: boolean;
	    isProxyDead: boolean;
	    killSwitchEmergency: boolean;
	    currentProxy?: ProxyConfig;
	    mode: string;
	    uptime: number;
	    bytesReceived: number;
	    bytesSent: number;
	    speedReceived: number;
	    speedSent: number;
	    killSwitchActive: boolean;
	
	    static createFrom(source: any = {}) {
	        return new StatusDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.isConnected = source["isConnected"];
	        this.isEstablishing = source["isEstablishing"];
	        this.isProxyDead = source["isProxyDead"];
	        this.killSwitchEmergency = source["killSwitchEmergency"];
	        this.currentProxy = this.convertValues(source["currentProxy"], ProxyConfig);
	        this.mode = source["mode"];
	        this.uptime = source["uptime"];
	        this.bytesReceived = source["bytesReceived"];
	        this.bytesSent = source["bytesSent"];
	        this.speedReceived = source["speedReceived"];
	        this.speedSent = source["speedSent"];
	        this.killSwitchActive = source["killSwitchActive"];
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

export namespace system {
	
	export class NetworkStatus {
	    online: boolean;
	    latency: number;
	    checkedAt: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new NetworkStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.online = source["online"];
	        this.latency = source["latency"];
	        this.checkedAt = source["checkedAt"];
	        this.error = source["error"];
	    }
	}
	export class TrafficStats {
	    received: number;
	    sent: number;
	
	    static createFrom(source: any = {}) {
	        return new TrafficStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.received = source["received"];
	        this.sent = source["sent"];
	    }
	}

}

export namespace updater {
	
	export class PlatformAsset {
	    url: string;
	    sha256: string;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new PlatformAsset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.sha256 = source["sha256"];
	        this.size = source["size"];
	    }
	}
	export class Manifest {
	    version: string;
	    downloadUrl: string;
	    platforms: Record<string, PlatformAsset>;
	
	    static createFrom(source: any = {}) {
	        return new Manifest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.downloadUrl = source["downloadUrl"];
	        this.platforms = this.convertValues(source["platforms"], PlatformAsset, true);
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

