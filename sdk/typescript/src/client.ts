import type {
  Agent,
  AgentVersion,
  CreateAgentRequest,
  CreateEnvironmentRequest,
  CreateSessionRequest,
  Environment,
  Event,
  Session,
  StoredEvent,
  UpdateAgentRequest,
  UserMessageEvent,
} from "./types.js";
import { ApiError } from "./types.js";

// ---------------------------------------------------------------------------
// Client options
// ---------------------------------------------------------------------------

export interface OmaClientOptions {
  /** Base URL of the OMA server (default: "http://localhost:8080") */
  baseUrl?: string;
  /** Optional API key for Bearer authentication */
  apiKey?: string;
}

// ---------------------------------------------------------------------------
// OmaClient
// ---------------------------------------------------------------------------

export class OmaClient {
  private readonly baseUrl: string;
  private readonly apiKey?: string;

  constructor(options: OmaClientOptions = {}) {
    this.baseUrl = (options.baseUrl ?? "http://localhost:8080").replace(
      /\/$/,
      "",
    );
    this.apiKey = options.apiKey;
  }

  // -----------------------------------------------------------------------
  // Internal helpers
  // -----------------------------------------------------------------------

  private headers(extra: Record<string, string> = {}): Record<string, string> {
    const h: Record<string, string> = { ...extra };
    if (this.apiKey) {
      h["Authorization"] = `Bearer ${this.apiKey}`;
    }
    return h;
  }

  private async request<T>(
    method: string,
    path: string,
    body?: unknown,
  ): Promise<T> {
    const url = `${this.baseUrl}${path}`;
    const init: RequestInit = {
      method,
      headers: this.headers(
        body !== undefined ? { "Content-Type": "application/json" } : {},
      ),
    };
    if (body !== undefined) {
      init.body = JSON.stringify(body);
    }

    const res = await fetch(url, init);

    if (!res.ok) {
      let errorBody: { error: string; message: string };
      try {
        errorBody = (await res.json()) as { error: string; message: string };
      } catch {
        errorBody = {
          error: "unknown",
          message: res.statusText || "request failed",
        };
      }
      throw new ApiError(res.status, errorBody);
    }

    // 204 No Content or similar
    const text = await res.text();
    if (!text) {
      return undefined as T;
    }
    return JSON.parse(text) as T;
  }

  // -----------------------------------------------------------------------
  // Agents
  // -----------------------------------------------------------------------

  async createAgent(req: CreateAgentRequest): Promise<Agent> {
    return this.request<Agent>("POST", "/v1/agents", req);
  }

  async listAgents(): Promise<Agent[]> {
    return this.request<Agent[]>("GET", "/v1/agents");
  }

  async getAgent(id: string): Promise<Agent> {
    return this.request<Agent>("GET", `/v1/agents/${encodeURIComponent(id)}`);
  }

  async updateAgent(id: string, req: UpdateAgentRequest): Promise<Agent> {
    return this.request<Agent>(
      "POST",
      `/v1/agents/${encodeURIComponent(id)}`,
      req,
    );
  }

  async archiveAgent(id: string): Promise<void> {
    await this.request<unknown>(
      "POST",
      `/v1/agents/${encodeURIComponent(id)}/archive`,
    );
  }

  async getAgentVersions(id: string): Promise<AgentVersion[]> {
    return this.request<AgentVersion[]>(
      "GET",
      `/v1/agents/${encodeURIComponent(id)}/versions`,
    );
  }

  // -----------------------------------------------------------------------
  // Environments
  // -----------------------------------------------------------------------

  async createEnvironment(req: CreateEnvironmentRequest): Promise<Environment> {
    return this.request<Environment>("POST", "/v1/environments", req);
  }

  async listEnvironments(): Promise<Environment[]> {
    return this.request<Environment[]>("GET", "/v1/environments");
  }

  async getEnvironment(id: string): Promise<Environment> {
    return this.request<Environment>(
      "GET",
      `/v1/environments/${encodeURIComponent(id)}`,
    );
  }

  async archiveEnvironment(id: string): Promise<void> {
    await this.request<unknown>(
      "POST",
      `/v1/environments/${encodeURIComponent(id)}/archive`,
    );
  }

  // -----------------------------------------------------------------------
  // Sessions
  // -----------------------------------------------------------------------

  async createSession(req: CreateSessionRequest): Promise<Session> {
    return this.request<Session>("POST", "/v1/sessions", req);
  }

  async listSessions(): Promise<Session[]> {
    return this.request<Session[]>("GET", "/v1/sessions");
  }

  async getSession(id: string): Promise<Session> {
    return this.request<Session>(
      "GET",
      `/v1/sessions/${encodeURIComponent(id)}`,
    );
  }

  async sendEvent(
    sessionId: string,
    event: UserMessageEvent,
  ): Promise<void> {
    await this.request<unknown>(
      "POST",
      `/v1/sessions/${encodeURIComponent(sessionId)}/events`,
      event,
    );
  }

  async getEvents(sessionId: string): Promise<StoredEvent[]> {
    return this.request<StoredEvent[]>(
      "GET",
      `/v1/sessions/${encodeURIComponent(sessionId)}/events`,
    );
  }

  // -----------------------------------------------------------------------
  // SSE streaming
  // -----------------------------------------------------------------------

  async *streamEvents(sessionId: string): AsyncIterableIterator<Event> {
    const url = `${this.baseUrl}/v1/sessions/${encodeURIComponent(sessionId)}/stream`;
    const res = await fetch(url, {
      method: "GET",
      headers: this.headers({ Accept: "text/event-stream" }),
    });

    if (!res.ok) {
      let errorBody: { error: string; message: string };
      try {
        errorBody = (await res.json()) as { error: string; message: string };
      } catch {
        errorBody = {
          error: "unknown",
          message: res.statusText || "stream failed",
        };
      }
      throw new ApiError(res.status, errorBody);
    }

    if (!res.body) {
      return;
    }

    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";

    try {
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });

        // SSE events are separated by double newlines
        const parts = buffer.split("\n\n");
        // Keep the last (possibly incomplete) chunk in the buffer
        buffer = parts.pop() ?? "";

        for (const part of parts) {
          const event = parseSseEvent(part);
          if (event) {
            yield event;
          }
        }
      }

      // Process any remaining buffer
      if (buffer.trim()) {
        const event = parseSseEvent(buffer);
        if (event) {
          yield event;
        }
      }
    } finally {
      reader.releaseLock();
    }
  }
}

// ---------------------------------------------------------------------------
// SSE parser helper
// ---------------------------------------------------------------------------

function parseSseEvent(raw: string): Event | null {
  let eventType = "message";
  let data = "";

  for (const line of raw.split("\n")) {
    if (line.startsWith("event:")) {
      eventType = line.slice("event:".length).trim();
    } else if (line.startsWith("data:")) {
      const payload = line.slice("data:".length).trim();
      data += data ? "\n" + payload : payload;
    }
    // Ignore id:, retry:, and comment lines (starting with :)
  }

  if (!data) {
    return null;
  }

  try {
    const content = JSON.parse(data) as unknown;
    return { type: eventType, content };
  } catch {
    // If data is not JSON, return it as a plain text content block
    return { type: eventType, content: data };
  }
}
