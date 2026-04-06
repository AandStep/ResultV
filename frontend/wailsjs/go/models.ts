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
	    }
	}
	export class AppSettings {
	    autostart: boolean;
	    killswitch: boolean;
	    adblock: boolean;
	    mode: string;
	    language: string;
	    theme: string;
	
	    static createFrom(source: any = {}) {
	        return new AppSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.autostart = source["autostart"];
	        this.killswitch = source["killswitch"];
	        this.adblock = source["adblock"];
	        this.mode = source["mode"];
	        this.language = source["language"];
	        this.theme = source["theme"];
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
	export class RoutingRules {
	    mode: string;
	    whitelist: string[];
	    appWhitelist: string[];
	
	    static createFrom(source: any = {}) {
	        return new RoutingRules(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.whitelist = source["whitelist"];
	        this.appWhitelist = source["appWhitelist"];
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

export namespace proxy {
	
	export class ConnectResultDTO {
	    success: boolean;
	    message: string;
	    gpoConflict: boolean;
	    tunnelFailed: boolean;
	    reason: string;
	    fallbackUsed: boolean;
	
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
	    }
	}
	export class PingResultDTO {
	    reachable: boolean;
	    latencyMs: number;
	
	    static createFrom(source: any = {}) {
	        return new PingResultDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.reachable = source["reachable"];
	        this.latencyMs = source["latencyMs"];
	    }
	}
	export class ProxyConfig {
	    ip: string;
	    port: number;
	    type: string;
	    username: string;
	    password: string;
	    uri?: string;
	    extra?: number[];
	
	    static createFrom(source: any = {}) {
	        return new ProxyConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ip = source["ip"];
	        this.port = source["port"];
	        this.type = source["type"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.uri = source["uri"];
	        this.extra = source["extra"];
	    }
	}
	export class StatusDTO {
	    isConnected: boolean;
	    isProxyDead: boolean;
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
	        this.isProxyDead = source["isProxyDead"];
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

