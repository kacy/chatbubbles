import type {
  Chat,
  CreateSessionResponse,
  Message,
  PairResponse,
  ServerInfo,
  SessionPollResponse,
} from './types';

type PairRequest = {
  code: string;
  clientName: string;
  clientType: 'web';
};

type SessionRequest = {
  clientName: string;
  clientType: 'web';
};

const requestTimeoutMs = 12000;

type ListMessagesOptions = {
  limit?: number;
  before?: string;
  attachments?: boolean;
};

export async function pairClient(
  apiBaseUrl: string,
  request: PairRequest,
): Promise<PairResponse> {
  return jsonRequest<PairResponse>(`${apiBaseUrl}/v1/pair`, {
    method: 'POST',
    body: JSON.stringify({
      code: request.code,
      client_name: request.clientName,
      client_type: request.clientType,
    }),
  });
}

export async function createSession(
  apiBaseUrl: string,
  request: SessionRequest,
): Promise<CreateSessionResponse> {
  return jsonRequest<CreateSessionResponse>(`${apiBaseUrl}/v1/sessions`, {
    method: 'POST',
    body: JSON.stringify({
      client_name: request.clientName,
      client_type: request.clientType,
    }),
  });
}

export async function pollSession(
  apiBaseUrl: string,
  sessionId: string,
): Promise<SessionPollResponse> {
  return jsonRequest<SessionPollResponse>(`${apiBaseUrl}/v1/sessions/${sessionId}`, {
    method: 'GET',
  });
}

export async function fetchServerInfo(
  apiBaseUrl: string,
  token: string,
): Promise<ServerInfo> {
  return jsonRequest<ServerInfo>(`${apiBaseUrl}/v1/server`, {
    method: 'GET',
    headers: {
      Authorization: `Bearer ${token}`,
    },
  });
}

export async function listChats(
  apiBaseUrl: string,
  token: string,
  limit = 50,
): Promise<Chat[]> {
  const query = new URLSearchParams({ limit: String(limit) });
  const response = await jsonRequest<{ chats: Chat[] }>(`${apiBaseUrl}/v1/chats?${query}`, {
    method: 'GET',
    headers: {
      Authorization: `Bearer ${token}`,
    },
  });

  return response.chats;
}

export async function listMessages(
  apiBaseUrl: string,
  token: string,
  chatId: number,
  options: ListMessagesOptions = {},
): Promise<Message[]> {
  const query = new URLSearchParams({
    limit: String(options.limit ?? 100),
  });
  if (options.before) {
    query.set('before', options.before);
  }
  if (options.attachments === false) {
    query.set('attachments', '0');
  }
  const response = await jsonRequest<{ messages: Message[] }>(
    `${apiBaseUrl}/v1/chats/${chatId}/messages?${query}`,
    {
      method: 'GET',
      headers: {
        Authorization: `Bearer ${token}`,
      },
    },
  );

  return response.messages;
}

async function jsonRequest<T>(url: string, init: RequestInit): Promise<T> {
  const controller = new AbortController();
  const timeout = window.setTimeout(() => controller.abort(), requestTimeoutMs);

  let response: Response;
  try {
    response = await fetch(url, {
      ...init,
      signal: controller.signal,
      headers: {
        'Content-Type': 'application/json',
        ...(init.headers ?? {}),
      },
    });
  } catch (error) {
    window.clearTimeout(timeout);
    if (error instanceof DOMException && error.name === 'AbortError') {
      throw new Error(`bridge request timed out after ${Math.round(requestTimeoutMs / 1000)}s`);
    }
    throw error;
  }
  window.clearTimeout(timeout);

  const raw = await response.text();
  let body: unknown = null;
  if (raw) {
    body = JSON.parse(raw);
  }

  if (!response.ok) {
    const message =
      typeof body === 'object' &&
      body !== null &&
      'error' in body &&
      typeof body.error === 'object' &&
      body.error !== null &&
      'message' in body.error
        ? String(body.error.message)
        : `request failed with status ${response.status}`;
    throw new Error(message);
  }

  return body as T;
}
