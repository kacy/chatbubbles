import type { ProfileDraft, StoredServerProfile } from './types';

const databaseName = 'imsg-bridge-web';
const databaseVersion = 1;
const profilesStore = 'profiles';
const appStateStore = 'app_state';

type AppStateRecord = {
  key: 'activeProfileId';
  value: string;
};

export async function listProfiles(): Promise<StoredServerProfile[]> {
  const db = await openDatabase();
  const tx = db.transaction(profilesStore, 'readonly');
  const store = tx.objectStore(profilesStore);
  const records = await request<StoredServerProfile[]>(store.getAll());
  return records.sort((left, right) => right.updatedAt.localeCompare(left.updatedAt));
}

export async function saveProfile(draft: ProfileDraft): Promise<StoredServerProfile> {
  const db = await openDatabase();
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
}

export async function deleteProfile(id: string): Promise<void> {
  const db = await openDatabase();
  const tx = db.transaction([profilesStore, appStateStore], 'readwrite');
  tx.objectStore(profilesStore).delete(id);
  const activeId = await request<string | undefined>(
    tx.objectStore(appStateStore).get('activeProfileId'),
  );
  if (activeId === id) {
    tx.objectStore(appStateStore).delete('activeProfileId');
  }
  await transactionDone(tx);
}

export async function setActiveProfile(id: string): Promise<void> {
  const db = await openDatabase();
  const tx = db.transaction(appStateStore, 'readwrite');
  const record: AppStateRecord = { key: 'activeProfileId', value: id };
  tx.objectStore(appStateStore).put(record);
  await transactionDone(tx);
}

export async function getActiveProfileId(): Promise<string | null> {
  const db = await openDatabase();
  const tx = db.transaction(appStateStore, 'readonly');
  const record = await request<AppStateRecord | undefined>(
    tx.objectStore(appStateStore).get('activeProfileId'),
  );
  return record?.value ?? null;
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
      if (!db.objectStoreNames.contains('chat_cache')) {
        db.createObjectStore('chat_cache', { keyPath: 'id' });
      }
      if (!db.objectStoreNames.contains('message_cache')) {
        db.createObjectStore('message_cache', { keyPath: 'id' });
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
