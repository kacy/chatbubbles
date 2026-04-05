export type PairPayload = {
  h: string;
  fp: string;
  c: string;
  v: number;
};

export type BrowserPairTarget = {
  bridgeHost: string;
  suggestedBrowserHost: string;
};

export type EncryptedValue = {
  cipherText: string;
  iv: string;
};

export type StoredServerProfile = {
  id: string;
  name: string;
  apiBaseUrl: string;
  wsBaseUrl: string;
  tlsFingerprint: string;
  token: EncryptedValue;
  scopes: string[];
  expiresAt: string;
  createdAt: string;
  updatedAt: string;
};

export type ProfileDraft = Omit<StoredServerProfile, 'id' | 'createdAt' | 'updatedAt'>;

export type PairResponse = {
  token: string;
  client_id: string;
  server_name: string;
  expires_at: string;
  scopes: string[];
};

export type CreateSessionResponse = {
  session_id: string;
  code: string;
  expires_at: string;
};

export type SessionPollResponse =
  | { status: 'pending' }
  | { status: 'expired' }
  | {
      status: 'approved';
      token: string;
      client_id: string;
      expires_at: string;
      scopes: string[];
    };

export type ServerInfo = {
  name: string;
  version: string;
  imsg_version: string;
  uptime_seconds: number;
  tailscale_ip?: string;
};
