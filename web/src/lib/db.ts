import type { Chat, Message, ProfileDraft, StoredServerProfile } from './types';

const databaseName = 'imsg-bridge-web';
const databaseVersion = 1;
const profilesStore = 'profiles';
const appStateStore = 'app_state';
const chatCacheStore = 'chat_cache';
const messageCacheStore = 'message_cache';

type AppStateRecord = {
  key: string;
  value: string;
};

type ChatCacheRecord = {
  id: string;
  profileId: string;
  chat: Chat;
  cachedAt: string;
};

type MessageCacheRecord = {
  id: string;
  profileId: string;
  chatId: number;
  messages: Message[];
  cachedAt: string;
};

export async function listProfiles(): Promise<StoredServerProfile[]> {
  const db = await openDatabase();
  try {
    const tx = db.transaction(profilesStore, 'readonly');
    const store = tx.objectStore(profilesStore);
    const records = await request<StoredServerProfile[]>(store.getAll());
    return records.sort((left, right) => right.updatedAt.localeCompare(left.updatedAt));
  } finally {
    db.close();
  }
}

export async function saveProfile(draft: ProfileDraft): Promise<StoredServerProfile> {
  const db = await openDatabase();
  try {
    const now = new Date().toISOString();
    const profile: StoredServerProfile = {
      id: crypto.randomUUID(),
      createdAt: now,
      updatedAt: now,
      ...draft,
    };

    const tx = db.transaction(profilesStore, 'readwrite');
    tx.objectStore(profilesStore).put(profile);
    await transactionDone(tx);
    return profile;
  } finally {
    db.close();
  }
}

export async function deleteProfile(id: string): Promise<void> {
  const db = await openDatabase();
  try {
    const tx = db.transaction(
      [profilesStore, appStateStore, chatCacheStore, messageCacheStore],
      'readwrite',
    );
    tx.objectStore(profilesStore).delete(id);
    const appState = tx.objectStore(appStateStore);
    const activeProfile = await request<AppStateRecord | undefined>(
      appState.get('activeProfileId'),
    );
    if (activeProfile?.value === id) {
      appState.delete('activeProfileId');
    }
    appState.delete(activeChatKey(id));
    await deleteCachedRecords<ChatCacheRecord>(tx.objectStore(chatCacheStore), (record) => record.profileId === id);
    await deleteCachedRecords<MessageCacheRecord>(
      tx.objectStore(messageCacheStore),
      (record) => record.profileId === id,
    );
    await transactionDone(tx);
  } finally {
    db.close();
  }
}

export async function setActiveProfile(id: string): Promise<void> {
  const db = await openDatabase();
  try {
    const tx = db.transaction(appStateStore, 'readwrite');
    const record: AppStateRecord = { key: 'activeProfileId', value: id };
    tx.objectStore(appStateStore).put(record);
    await transactionDone(tx);
  } finally {
    db.close();
  }
}

export async function getActiveProfileId(): Promise<string | null> {
  const db = await openDatabase();
  try {
    const tx = db.transaction(appStateStore, 'readonly');
    const record = await request<AppStateRecord | undefined>(
      tx.objectStore(appStateStore).get('activeProfileId'),
    );
    return record?.value ?? null;
  } finally {
    db.close();
  }
}

export async function setActiveChat(profileId: string, chatId: number): Promise<void> {
  const db = await openDatabase();
  try {
    const tx = db.transaction(appStateStore, 'readwrite');
    const record: AppStateRecord = { key: activeChatKey(profileId), value: String(chatId) };
    tx.objectStore(appStateStore).put(record);
    await transactionDone(tx);
  } finally {
    db.close();
  }
}

export async function getActiveChatId(profileId: string): Promise<number | null> {
  const db = await openDatabase();
  try {
    const tx = db.transaction(appStateStore, 'readonly');
    const record = await request<AppStateRecord | undefined>(
      tx.objectStore(appStateStore).get(activeChatKey(profileId)),
    );
    if (!record?.value) {
      return null;
    }

    const value = Number.parseInt(record.value, 10);
    return Number.isFinite(value) ? value : null;
  } finally {
    db.close();
  }
}

export async function getCachedChats(profileId: string): Promise<Chat[]> {
  const db = await openDatabase();
  try {
    const tx = db.transaction(chatCacheStore, 'readonly');
    const records = await request<ChatCacheRecord[]>(tx.objectStore(chatCacheStore).getAll());
    return records
      .filter((record) => record.profileId === profileId)
      .map((record) => record.chat);
  } finally {
    db.close();
  }
}

export async function saveChats(profileId: string, chats: Chat[]): Promise<void> {
  const db = await openDatabase();
  try {
    const tx = db.transaction(chatCacheStore, 'readwrite');
    const store = tx.objectStore(chatCacheStore);
    await deleteCachedRecords<ChatCacheRecord>(store, (record) => record.profileId === profileId);

    const cachedAt = new Date().toISOString();
    for (const chat of chats) {
      const record: ChatCacheRecord = {
        id: chatCacheKey(profileId, chat.id),
        profileId,
        chat,
        cachedAt,
      };
      store.put(record);
    }

    await transactionDone(tx);
  } finally {
    db.close();
  }
}

export async function getCachedMessages(profileId: string, chatId: number): Promise<Message[]> {
  const db = await openDatabase();
  try {
    const tx = db.transaction(messageCacheStore, 'readonly');
    const record = await request<MessageCacheRecord | undefined>(
      tx.objectStore(messageCacheStore).get(messageCacheKey(profileId, chatId)),
    );
    return record?.messages ?? [];
  } finally {
    db.close();
  }
}

export async function saveMessages(
  profileId: string,
  chatId: number,
  messages: Message[],
): Promise<void> {
  const db = await openDatabase();
  try {
    const tx = db.transaction(messageCacheStore, 'readwrite');
    const record: MessageCacheRecord = {
      id: messageCacheKey(profileId, chatId),
      profileId,
      chatId,
      messages,
      cachedAt: new Date().toISOString(),
    };
    tx.objectStore(messageCacheStore).put(record);
    await transactionDone(tx);
  } finally {
    db.close();
  }
}

async function openDatabase(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(databaseName, databaseVersion);

    req.onupgradeneeded = () => {
      const db = req.result;
      if (!db.objectStoreNames.contains(profilesStore)) {
        db.createObjectStore(profilesStore, { keyPath: 'id' });
      }
      if (!db.objectStoreNames.contains(appStateStore)) {
        db.createObjectStore(appStateStore, { keyPath: 'key' });
      }
      if (!db.objectStoreNames.contains(chatCacheStore)) {
        db.createObjectStore(chatCacheStore, { keyPath: 'id' });
      }
      if (!db.objectStoreNames.contains(messageCacheStore)) {
        db.createObjectStore(messageCacheStore, { keyPath: 'id' });
      }
      if (!db.objectStoreNames.contains('attachment_cache')) {
        db.createObjectStore('attachment_cache', { keyPath: 'id' });
      }
    };

    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

function request<T>(req: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

function transactionDone(tx: IDBTransaction): Promise<void> {
  return new Promise((resolve, reject) => {
    tx.oncomplete = () => resolve();
    tx.onabort = () => reject(tx.error);
    tx.onerror = () => reject(tx.error);
  });
}

function activeChatKey(profileId: string): string {
  return `activeChat:${profileId}`;
}

function chatCacheKey(profileId: string, chatId: number): string {
  return `${profileId}:chat:${chatId}`;
}

function messageCacheKey(profileId: string, chatId: number): string {
  return `${profileId}:messages:${chatId}`;
}

async function deleteCachedRecords<T extends { id: string }>(
  store: IDBObjectStore,
  predicate: (record: T) => boolean,
): Promise<void> {
  const records = await request<T[]>(store.getAll());
  for (const record of records) {
    if (predicate(record)) {
      store.delete(record.id);
    }
  }
}
