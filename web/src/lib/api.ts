import type {
  CreateSessionResponse,
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

async function jsonRequest<T>(url: string, init: RequestInit): Promise<T> {
  const response = await fetch(url, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(init.headers ?? {}),
    },
  });

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
