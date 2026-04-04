import { describe, expect, test } from 'vitest';

import { buildProfileDraft, deriveApiBaseUrl, deriveWsBaseUrl } from './connection';

describe('connection helpers', () => {
  test('normalizes host into https api base url', () => {
    expect(deriveApiBaseUrl('100.64.0.3:8443')).toBe('https://100.64.0.3:8443');
  });

  test('derives websocket url from the saved api base', () => {
    expect(deriveWsBaseUrl('https://100.64.0.3:8443')).toBe('wss://100.64.0.3:8443');
  });

  test('builds profile urls from the saved host instead of browser origin', () => {
    const profile = buildProfileDraft({
      name: 'home bridge',
      host: 'bridge.example.ts.net:8443',
      tlsFingerprint: 'SHA256:test',
      token: { cipherText: 'a', iv: 'b' },
      scopes: ['read'],
      expiresAt: '2026-04-04T00:00:00Z',
    });

    expect(profile.apiBaseUrl).toBe('https://bridge.example.ts.net:8443');
    expect(profile.wsBaseUrl).toBe('wss://bridge.example.ts.net:8443');
  });
});
