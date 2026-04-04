import type { EncryptedValue } from './types';

const keyStorageKey = 'imsg-bridge.web.crypto-key';

export async function encryptString(plainText: string): Promise<EncryptedValue> {
  const key = await getOrCreateKey();
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const encoded = new TextEncoder().encode(plainText);
  const cipherBuffer = await crypto.subtle.encrypt(
    { name: 'AES-GCM', iv },
    key,
    encoded,
  );

  return {
    cipherText: toBase64(cipherBuffer),
    iv: toBase64(iv),
  };
}

export async function decryptString(value: EncryptedValue): Promise<string> {
  const key = await getOrCreateKey();
  const iv = toArrayBuffer(fromBase64(value.iv));
  const cipherText = toArrayBuffer(fromBase64(value.cipherText));
  const plainBuffer = await crypto.subtle.decrypt(
    { name: 'AES-GCM', iv },
    key,
    cipherText,
  );

  return new TextDecoder().decode(plainBuffer);
}

async function getOrCreateKey(): Promise<CryptoKey> {
  const existing = globalThis.localStorage?.getItem(keyStorageKey);
  if (existing) {
    return crypto.subtle.importKey(
      'raw',
      toArrayBuffer(fromBase64(existing)),
      { name: 'AES-GCM' },
      false,
      ['encrypt', 'decrypt'],
    );
  }

  const key = await crypto.subtle.generateKey(
    { name: 'AES-GCM', length: 256 },
    true,
    ['encrypt', 'decrypt'],
  );
  const raw = await crypto.subtle.exportKey('raw', key);
  globalThis.localStorage?.setItem(keyStorageKey, toBase64(raw));
  return crypto.subtle.importKey('raw', raw, { name: 'AES-GCM' }, false, [
    'encrypt',
    'decrypt',
  ]);
}

function toBase64(value: ArrayBuffer | Uint8Array): string {
  const bytes = value instanceof Uint8Array ? value : new Uint8Array(value);
  let text = '';
  bytes.forEach((byte) => {
    text += String.fromCharCode(byte);
  });
  return btoa(text);
}

function fromBase64(value: string): Uint8Array {
  const text = atob(value);
  const bytes = new Uint8Array(text.length);
  for (let index = 0; index < text.length; index += 1) {
    bytes[index] = text.charCodeAt(index);
  }
  return bytes;
}

function toArrayBuffer(value: Uint8Array): ArrayBuffer {
  return value.buffer.slice(
    value.byteOffset,
    value.byteOffset + value.byteLength,
  ) as ArrayBuffer;
}
