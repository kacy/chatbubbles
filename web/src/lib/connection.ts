import type { BrowserPairTarget, ProfileDraft } from './types';

export function deriveApiBaseUrl(host: string): string {
  const trimmed = host.trim();
  if (!trimmed) {
    throw new Error('server host is required');
  }

  const withScheme = /^https?:\/\//i.test(trimmed) ? trimmed : `https://${trimmed}`;
  const url = new URL(withScheme);

  if (url.protocol !== 'https:') {
    throw new Error('server host must use https');
  }

  url.pathname = '';
  url.search = '';
  url.hash = '';

  return url.toString().replace(/\/$/, '');
}

export function deriveWsBaseUrl(apiBaseUrl: string): string {
  const url = new URL(apiBaseUrl);
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  return url.toString().replace(/\/$/, '');
}

export function buildProfileDraft(input: {
  name: string;
  host: string;
  tlsFingerprint: string;
  token: ProfileDraft['token'];
  scopes: string[];
  expiresAt: string;
}): ProfileDraft {
  const apiBaseUrl = deriveApiBaseUrl(input.host);

  return {
    name: input.name.trim(),
    apiBaseUrl,
    wsBaseUrl: deriveWsBaseUrl(apiBaseUrl),
    tlsFingerprint: input.tlsFingerprint.trim(),
    token: input.token,
    scopes: input.scopes,
    expiresAt: input.expiresAt,
  };
}

export function deriveBrowserPairTarget(host: string): BrowserPairTarget {
  const trimmed = host.trim();
  if (!trimmed) {
    throw new Error('server host is required');
  }

  const normalized = trimmed.replace(/^https?:\/\//i, '').replace(/\/+$/, '');
  const browserHost = stripDefaultPort(normalized);

  return {
    bridgeHost: normalized,
    suggestedBrowserHost: browserHost,
  };
}

function stripDefaultPort(host: string): string {
  if (host.endsWith(':443')) {
    return host.slice(0, -4);
  }
  return host;
}
