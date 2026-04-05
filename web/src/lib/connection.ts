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
  const browserHost = browserSafeHost(normalized);

  return {
    bridgeHost: normalized,
    suggestedBrowserHost: browserHost,
  };
}

export function requireBrowserSafeHost(host: string): string {
  const trimmed = host.trim();
  if (!trimmed) {
    throw new Error('browser host is required');
  }

  const normalized = trimmed.replace(/^https?:\/\//i, '').replace(/\/+$/, '');
  const safeHost = browserSafeHost(normalized);
  if (!safeHost) {
    throw new Error(
      'browser host must be a browser-trusted https hostname, like bridge.your-tailnet.ts.net, not a raw ip or :8443 endpoint',
    );
  }

  return safeHost;
}

function browserSafeHost(host: string): string {
  const withScheme = /^https?:\/\//i.test(host) ? host : `https://${host}`;
  const url = new URL(withScheme);

  if (url.protocol !== 'https:') {
    return '';
  }

  if (url.username || url.password || url.pathname !== '/' || url.search || url.hash) {
    return '';
  }

  const hostname = url.hostname.trim();
  if (!hostname || isIPAddress(hostname)) {
    return '';
  }

  if (url.port && url.port !== '443') {
    return '';
  }

  return url.host;
}

function isIPAddress(value: string): boolean {
  if (/^\d{1,3}(?:\.\d{1,3}){3}$/.test(value)) {
    return true;
  }

  return value.includes(':');
}
