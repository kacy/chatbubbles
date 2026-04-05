import { beforeEach, describe, expect, test } from 'vitest';

import { decryptString, encryptString } from './crypto';
import {
  getCachedChats,
  getCachedMessages,
  getActiveChatId,
  getActiveProfileId,
  listProfiles,
  saveProfile,
  saveChats,
  saveMessages,
  setActiveChat,
  setActiveProfile,
} from './db';

describe('persistence', () => {
  beforeEach(() => {
    window.localStorage.clear();
    indexedDB.deleteDatabase('chatbubbles-web');
  });

  test('encrypts and decrypts token values', async () => {
    const encrypted = await encryptString('secret-token');
    const decrypted = await decryptString(encrypted);

    expect(decrypted).toBe('secret-token');
  });

  test('saves and restores server profiles', async () => {
    const profile = await saveProfile({
      name: 'home',
      apiBaseUrl: 'https://100.64.0.3:8443',
      wsBaseUrl: 'wss://100.64.0.3:8443',
      tlsFingerprint: 'SHA256:test',
      token: await encryptString('secret-token'),
      scopes: ['read', 'send'],
      expiresAt: '2026-07-03T00:00:00Z',
    });

    await setActiveProfile(profile.id);

    const profiles = await listProfiles();
    expect(profiles).toHaveLength(1);
    expect(profiles[0].name).toBe('home');
    expect(await getActiveProfileId()).toBe(profile.id);
  });

  test('stores the last active chat per profile', async () => {
    const profile = await saveProfile({
      name: 'home',
      apiBaseUrl: 'https://bridge.example.ts.net',
      wsBaseUrl: 'wss://bridge.example.ts.net',
      tlsFingerprint: 'SHA256:test',
      token: await encryptString('secret-token'),
      scopes: ['read', 'send'],
      expiresAt: '2026-07-03T00:00:00Z',
    });

    await setActiveChat(profile.id, 42);

    expect(await getActiveChatId(profile.id)).toBe(42);
  });

  test('stores cached chats and per-thread messages', async () => {
    const profile = await saveProfile({
      name: 'home',
      apiBaseUrl: 'https://bridge.example.ts.net',
      wsBaseUrl: 'wss://bridge.example.ts.net',
      tlsFingerprint: 'SHA256:test',
      token: await encryptString('secret-token'),
      scopes: ['read', 'send'],
      expiresAt: '2026-07-03T00:00:00Z',
    });

    await saveChats(profile.id, [
      {
        id: 7,
        name: 'family',
        identifier: 'chat123',
        service: 'iMessage',
        last_message_at: '2026-04-04T12:00:00Z',
      },
    ]);
    await saveMessages(profile.id, 7, [
      {
        id: 99,
        chat_id: 7,
        text: 'cached hello',
        is_from_me: false,
        created_at: '2026-04-04T12:00:00Z',
      },
    ]);

    expect(await getCachedChats(profile.id)).toEqual([
      {
        id: 7,
        name: 'family',
        identifier: 'chat123',
        service: 'iMessage',
        last_message_at: '2026-04-04T12:00:00Z',
      },
    ]);
    expect(await getCachedMessages(profile.id, 7)).toEqual([
      {
        id: 99,
        chat_id: 7,
        text: 'cached hello',
        is_from_me: false,
        created_at: '2026-04-04T12:00:00Z',
      },
    ]);
  });
});
