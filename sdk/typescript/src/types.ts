// ---------------------------------------------------------------------------
// Agent types
// ---------------------------------------------------------------------------

export interface Agent {
  id: string;
  name: string;
  model: ModelConfig;
  system?: string;
  tools: ToolConfig[];
  mcp_servers?: McpServerConfig[];
  skills?: SkillConfig[];
  callable_agents?: string[];
  description?: string;
  metadata?: Record<string, string>;
  version: number;
  created_at: string;
  updated_at: string;
  archived_at?: string;
}

export interface ModelConfig {
  id: string;
  speed?: string;
}

export interface ToolConfig {
  type: string;
  default_config?: DefaultConfig;
}

export interface DefaultConfig {
  permission_policy?: PermissionPolicy;
}

export interface PermissionPolicy {
  type: string;
  scopes?: ToolScope[];
}

export interface ToolScope {
  tool: string;
  allow: boolean;
  constraints?: Constraints;
}

export interface Constraints {
  paths?: string[];
  commands?: string[];
  domains?: string[];
}

export interface McpServerConfig {
  name: string;
  url: string;
  auth?: unknown;
}

export interface SkillConfig {
  name: string;
  description?: string;
}

// ---------------------------------------------------------------------------
// Environment types
// ---------------------------------------------------------------------------

export interface Environment {
  id: string;
  name: string;
  config: EnvironmentConfig;
  created_at: string;
  updated_at: string;
  archived_at?: string;
}

export interface EnvironmentConfig {
  type: string;
  networking: NetworkConfig;
  packages?: string[];
  env_vars?: Record<string, string>;
  resources?: Resources;
}

export interface NetworkConfig {
  type: string;
  allowed_domains?: string[];
}

export interface Resources {
  memory_mb?: number;
  cpu_cores?: number;
  timeout_seconds?: number;
}

// ---------------------------------------------------------------------------
// Session types
// ---------------------------------------------------------------------------

export type SessionStatus =
  | "starting"
  | "running"
  | "idle"
  | "paused"
  | "completed"
  | "failed";

export interface Session {
  id: string;
  agent: string;
  agent_version: number;
  environment_id: string;
  title?: string;
  status: SessionStatus;
  metadata?: Record<string, string>;
  created_at: string;
  updated_at: string;
  completed_at?: string;
}

// ---------------------------------------------------------------------------
// Event types
// ---------------------------------------------------------------------------

export interface Event {
  type: string;
  content?: unknown;
}

export interface ContentBlock {
  type: string;
  text: string;
}

export interface ToolUseEvent {
  type: string;
  id: string;
  name: string;
  input: unknown;
}

export interface UserMessageEvent {
  type: string;
  content: ContentBlock[];
}

export interface StoredEvent {
  id: string;
  session_id: string;
  type: string;
  data: unknown;
  created_at: string;
}

// ---------------------------------------------------------------------------
// Version types
// ---------------------------------------------------------------------------

export interface AgentVersion {
  agent_id: string;
  version: number;
  config: unknown;
  created_at: string;
}

// ---------------------------------------------------------------------------
// Request types
// ---------------------------------------------------------------------------

export interface CreateAgentRequest {
  name: string;
  model: ModelConfig;
  system?: string;
  tools?: ToolConfig[];
  mcp_servers?: McpServerConfig[];
  skills?: SkillConfig[];
  callable_agents?: string[];
  description?: string;
  metadata?: Record<string, string>;
}

export interface UpdateAgentRequest {
  version: number;
  name?: string;
  model?: ModelConfig;
  system?: string;
  tools?: ToolConfig[];
  mcp_servers?: McpServerConfig[];
  skills?: SkillConfig[];
  callable_agents?: string[];
  description?: string;
  metadata?: Record<string, string>;
}

export interface CreateEnvironmentRequest {
  name: string;
  config: EnvironmentConfig;
}

export interface CreateSessionRequest {
  agent_id: string;
  environment_id: string;
  title?: string;
  metadata?: Record<string, string>;
}

// ---------------------------------------------------------------------------
// Error types
// ---------------------------------------------------------------------------

export interface ApiErrorBody {
  error: string;
  message: string;
}

export class ApiError extends Error {
  public readonly status: number;
  public readonly code: string;
  public readonly body: ApiErrorBody;

  constructor(status: number, body: ApiErrorBody) {
    super(`${body.error}: ${body.message}`);
    this.name = "ApiError";
    this.status = status;
    this.code = body.error;
    this.body = body;
  }
}
