export namespace config {
	
	export class MCPServer {
	    Name: string;
	    Command: string;
	    Args: string[];
	    Env: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new MCPServer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Name = source["Name"];
	        this.Command = source["Command"];
	        this.Args = source["Args"];
	        this.Env = source["Env"];
	    }
	}
	export class PlatformBotConfig {
	    Enabled: boolean;
	    Token: string;
	    AllowedUsers: string[];
	    BlockedUsers: string[];
	    GuildIDs: string[];
	    AllowedRoles: string[];
	    RateLimit: number;
	    RateBurst: number;
	
	    static createFrom(source: any = {}) {
	        return new PlatformBotConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Enabled = source["Enabled"];
	        this.Token = source["Token"];
	        this.AllowedUsers = source["AllowedUsers"];
	        this.BlockedUsers = source["BlockedUsers"];
	        this.GuildIDs = source["GuildIDs"];
	        this.AllowedRoles = source["AllowedRoles"];
	        this.RateLimit = source["RateLimit"];
	        this.RateBurst = source["RateBurst"];
	    }
	}
	export class WhatsAppConfig {
	    Enabled: boolean;
	    SessionDir: string;
	    AllowedNumbers: string[];
	    RateLimit: number;
	    RateBurst: number;
	
	    static createFrom(source: any = {}) {
	        return new WhatsAppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Enabled = source["Enabled"];
	        this.SessionDir = source["SessionDir"];
	        this.AllowedNumbers = source["AllowedNumbers"];
	        this.RateLimit = source["RateLimit"];
	        this.RateBurst = source["RateBurst"];
	    }
	}
	export class PlatformConfig {
	    Telegram: PlatformBotConfig;
	    Discord: PlatformBotConfig;
	    WhatsApp: WhatsAppConfig;
	    MaxResponseLen: number;
	    TypingIndicator: boolean;
	    LogConversations: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PlatformConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Telegram = this.convertValues(source["Telegram"], PlatformBotConfig);
	        this.Discord = this.convertValues(source["Discord"], PlatformBotConfig);
	        this.WhatsApp = this.convertValues(source["WhatsApp"], WhatsAppConfig);
	        this.MaxResponseLen = source["MaxResponseLen"];
	        this.TypingIndicator = source["TypingIndicator"];
	        this.LogConversations = source["LogConversations"];
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
	export class SmaraConfig {
	    Provider: string;
	    Model: string;
	    OllamaHost: string;
	    OpenAIAPIKey: string;
	    OpenAIModel: string;
	    OpenAIBaseURL: string;
	    OpenRouterAPIKey: string;
	    OpenRouterModel: string;
	    AnthropicAPIKey: string;
	    AnthropicModel: string;
	    CustomProviderName: string;
	    CustomAPIKey: string;
	    CustomBaseURL: string;
	    CustomModel: string;
	    SyncDir: string;
	    SyncInterval: number;
	    MCPServers: MCPServer[];
	    SmaraMCPEnabled: boolean;
	    SmaraMCPCommand: string;
	    SmaraMCPArgs: string[];
	    SmaraMCPAPIKey: string;
	    Verbose: boolean;
	    DBPath: string;
	    ActiveWorkspace: string;
	    ActiveWorkspaceID: number;
	    Platforms: PlatformConfig;
	
	    static createFrom(source: any = {}) {
	        return new SmaraConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Provider = source["Provider"];
	        this.Model = source["Model"];
	        this.OllamaHost = source["OllamaHost"];
	        this.OpenAIAPIKey = source["OpenAIAPIKey"];
	        this.OpenAIModel = source["OpenAIModel"];
	        this.OpenAIBaseURL = source["OpenAIBaseURL"];
	        this.OpenRouterAPIKey = source["OpenRouterAPIKey"];
	        this.OpenRouterModel = source["OpenRouterModel"];
	        this.AnthropicAPIKey = source["AnthropicAPIKey"];
	        this.AnthropicModel = source["AnthropicModel"];
	        this.CustomProviderName = source["CustomProviderName"];
	        this.CustomAPIKey = source["CustomAPIKey"];
	        this.CustomBaseURL = source["CustomBaseURL"];
	        this.CustomModel = source["CustomModel"];
	        this.SyncDir = source["SyncDir"];
	        this.SyncInterval = source["SyncInterval"];
	        this.MCPServers = this.convertValues(source["MCPServers"], MCPServer);
	        this.SmaraMCPEnabled = source["SmaraMCPEnabled"];
	        this.SmaraMCPCommand = source["SmaraMCPCommand"];
	        this.SmaraMCPArgs = source["SmaraMCPArgs"];
	        this.SmaraMCPAPIKey = source["SmaraMCPAPIKey"];
	        this.Verbose = source["Verbose"];
	        this.DBPath = source["DBPath"];
	        this.ActiveWorkspace = source["ActiveWorkspace"];
	        this.ActiveWorkspaceID = source["ActiveWorkspaceID"];
	        this.Platforms = this.convertValues(source["Platforms"], PlatformConfig);
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

export namespace llm {
	
	export class ToolCall {
	    id: string;
	    function: string;
	    arguments: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new ToolCall(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.function = source["function"];
	        this.arguments = source["arguments"];
	    }
	}
	export class Message {
	    role: string;
	    content: string;
	    tool_call_id?: string;
	    tool_calls?: ToolCall[];
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	        this.tool_call_id = source["tool_call_id"];
	        this.tool_calls = this.convertValues(source["tool_calls"], ToolCall);
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

export namespace memory {
	
	export class Workspace {
	    id: number;
	    name: string;
	    path: string;
	    // Go type: time
	    created_at: any;
	
	    static createFrom(source: any = {}) {
	        return new Workspace(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.created_at = this.convertValues(source["created_at"], null);
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

export namespace session {
	
	export class Task {
	    id: string;
	    description: string;
	    status: string;
	    assigned_to?: string;
	    parent_id?: string;
	    mcp_server?: string;
	    tool_name?: string;
	    tool_args?: Record<string, any>;
	    // Go type: time
	    created_at: any;
	
	    static createFrom(source: any = {}) {
	        return new Task(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.description = source["description"];
	        this.status = source["status"];
	        this.assigned_to = source["assigned_to"];
	        this.parent_id = source["parent_id"];
	        this.mcp_server = source["mcp_server"];
	        this.tool_name = source["tool_name"];
	        this.tool_args = source["tool_args"];
	        this.created_at = this.convertValues(source["created_at"], null);
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
	export class Session {
	    id: string;
	    workspace_id: number;
	    name: string;
	    state: string;
	    mode: string;
	    mcp_servers: string[];
	    history: llm.Message[];
	    tasks: Task[];
	    memory_ids: number[];
	    context: string;
	    is_agentic: boolean;
	    auto_resume: boolean;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new Session(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.workspace_id = source["workspace_id"];
	        this.name = source["name"];
	        this.state = source["state"];
	        this.mode = source["mode"];
	        this.mcp_servers = source["mcp_servers"];
	        this.history = this.convertValues(source["history"], llm.Message);
	        this.tasks = this.convertValues(source["tasks"], Task);
	        this.memory_ids = source["memory_ids"];
	        this.context = source["context"];
	        this.is_agentic = source["is_agentic"];
	        this.auto_resume = source["auto_resume"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
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

