import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { Link, Navigate, Route, Routes, useNavigate } from 'react-router-dom';

import {
  createSession,
  fetchServerInfo,
  listChats,
  listMessages,
  pairClient,
  pollSession,
} from './lib/api';
import { decryptString, encryptString } from './lib/crypto';
import {
  deleteProfile,
  getCachedChats,
  getCachedMessages,
  getActiveChatId,
  getActiveProfileId,
  listProfiles,
  saveChats,
  saveMessages,
  saveProfile,
  setActiveChat,
  setActiveProfile,
} from './lib/db';
import {
  buildProfileDraft,
  deriveApiBaseUrl,
  deriveBrowserPairTarget,
  requireBrowserSafeHost,
} from './lib/connection';
import { parsePairPayload } from './lib/qr';
import type {
  BridgeEvent,
  Chat,
  CreateSessionResponse,
  Message,
  PairPayload,
  ServerInfo,
  SessionPollResponse,
  StoredServerProfile,
} from './lib/types';

type AppState = {
  profiles: StoredServerProfile[];
  activeProfileId: string | null;
  loading: boolean;
};

const recentMessageLimit = 20;
const olderMessagePageSize = 60;

function defaultClientName() {
  return `browser on ${navigator.platform || 'this device'}`;
}

// ---------------------------------------------------------------------------
// app root
// ---------------------------------------------------------------------------

function App() {
  const [state, setState] = useState<AppState>({
    profiles: [],
    activeProfileId: null,
    loading: true,
  });

  useEffect(() => {
    loadState().then(setState).catch((error) => {
      console.error('failed to load web shell state', error);
      setState((current) => ({ ...current, loading: false }));
    });
  }, []);

  const activeProfile = useMemo(
    () => state.profiles.find((profile) => profile.id === state.activeProfileId) ?? null,
    [state.activeProfileId, state.profiles],
  );

  async function refreshState() {
    setState(await loadState());
  }

  async function saveAndActivateProfile(input: {
    profileName: string;
    host: string;
    tlsFingerprint: string;
    token: string;
    scopes: string[];
    expiresAt: string;
  }) {
    const encryptedToken = await encryptString(input.token);
    const profile = await saveProfile(
      buildProfileDraft({
        name: input.profileName,
        host: input.host,
        tlsFingerprint: input.tlsFingerprint,
        token: encryptedToken,
        scopes: input.scopes,
        expiresAt: input.expiresAt,
      }),
    );
    await setActiveProfile(profile.id);
    await refreshState();
  }

  async function activateProfile(id: string) {
    await setActiveProfile(id);
    await refreshState();
  }

  async function removeProfile(id: string) {
    await deleteProfile(id);
    await refreshState();
  }

  if (state.loading) {
    return (
      <div className="flex h-dvh items-center justify-center">
        <div className="text-center">
          <div className="mx-auto h-8 w-8 animate-spin rounded-full border-2 border-slate-200 border-t-signal" />
          <p className="mt-3 text-sm text-slate-500">loading…</p>
        </div>
      </div>
    );
  }

  return (
    <Routes>
      <Route
        path="/"
        element={
          <HomePage
            profiles={state.profiles}
            activeProfileId={state.activeProfileId}
            onActivate={activateProfile}
            onDelete={removeProfile}
          />
        }
      />
      <Route
        path="/pair"
        element={<PairPage onSaveProfile={saveAndActivateProfile} />}
      />
      <Route
        path="/session"
        element={<SessionPage onSaveProfile={saveAndActivateProfile} />}
      />
      <Route
        path="/settings"
        element={
          activeProfile ? (
            <SettingsPage profile={activeProfile} />
          ) : (
            <Navigate replace to="/" />
          )
        }
      />
      <Route
        path="/app"
        element={
          activeProfile ? (
            <AppShell profile={activeProfile} />
          ) : (
            <Navigate replace to="/" />
          )
        }
      />
    </Routes>
  );
}

// ---------------------------------------------------------------------------
// simple page toolbar
// ---------------------------------------------------------------------------

function PageToolbar({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="toolbar">
      <Link to="/" className="text-slate-400 hover:text-slate-600">
        <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="h-5 w-5">
          <path fillRule="evenodd" d="M17 10a.75.75 0 01-.75.75H5.612l4.158 3.96a.75.75 0 11-1.04 1.08l-5.5-5.25a.75.75 0 010-1.08l5.5-5.25a.75.75 0 111.04 1.08L5.612 9.25H16.25A.75.75 0 0117 10z" clipRule="evenodd" />
        </svg>
      </Link>
      <h1 className="text-sm font-semibold text-slate-900">{title}</h1>
      <div className="ml-auto flex items-center gap-2">{children}</div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// home page
// ---------------------------------------------------------------------------

function HomePage(props: {
  profiles: StoredServerProfile[];
  activeProfileId: string | null;
  onActivate: (id: string) => Promise<void>;
  onDelete: (id: string) => Promise<void>;
}) {
  return (
    <div className="flex min-h-dvh flex-col">
      <div className="toolbar">
        <h1 className="text-sm font-semibold text-slate-900">imsg-bridge</h1>
      </div>

      <div className="mx-auto w-full max-w-lg flex-1 px-4 py-6">
        {props.profiles.length === 0 ? (
          <div className="mt-12 text-center">
            <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-2xl bg-signal text-xl font-bold text-white">
              i
            </div>
            <h2 className="mt-4 text-lg font-semibold text-slate-900">connect a bridge</h2>
            <p className="mt-2 text-sm text-slate-500">
              pair this browser with your home imsg-bridge server.
            </p>
            <div className="mt-6 flex justify-center gap-3">
              <Link className="button-primary" to="/pair">pair with qr</Link>
              <Link className="button-secondary" to="/session">approval code</Link>
            </div>
          </div>
        ) : (
          <>
            <div className="flex items-center justify-between">
              <h2 className="text-sm font-semibold text-slate-900">bridges</h2>
              <div className="flex gap-2">
                <Link className="button-secondary !px-3 !py-1.5 text-xs" to="/pair">pair</Link>
                <Link className="button-secondary !px-3 !py-1.5 text-xs" to="/session">session</Link>
              </div>
            </div>

            <div className="mt-4 space-y-2">
              {props.profiles.map((profile) => {
                const isActive = profile.id === props.activeProfileId;
                return (
                  <div
                    key={profile.id}
                    className={
                      isActive
                        ? 'rounded-xl border border-signal/20 bg-signal/5 p-4'
                        : 'rounded-xl border border-slate-200 bg-white p-4'
                    }
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <p className="truncate text-sm font-semibold text-slate-900">
                          {profile.name}
                        </p>
                        <p className="mt-1 truncate text-xs text-slate-500">
                          {profile.apiBaseUrl}
                        </p>
                        <p className="mt-1 text-xs text-slate-400">
                          {profile.scopes.join(' · ')}
                        </p>
                      </div>
                      <div className="flex shrink-0 gap-2">
                        <button
                          className="rounded-lg border border-slate-200 px-2.5 py-1.5 text-xs text-slate-600 hover:bg-slate-50"
                          onClick={() => void props.onDelete(profile.id)}
                          type="button"
                        >
                          remove
                        </button>
                        <button
                          className={
                            isActive
                              ? 'rounded-lg bg-signal/10 px-2.5 py-1.5 text-xs font-medium text-signal'
                              : 'rounded-lg bg-signal px-2.5 py-1.5 text-xs font-medium text-white'
                          }
                          onClick={() => void props.onActivate(profile.id)}
                          type="button"
                        >
                          {isActive ? 'active' : 'use'}
                        </button>
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>

            {props.activeProfileId ? (
              <div className="mt-6">
                <Link className="button-primary w-full justify-center" to="/app">
                  open messages
                </Link>
              </div>
            ) : null}
          </>
        )}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// pair page
// ---------------------------------------------------------------------------

function PairPage(props: {
  onSaveProfile: (input: {
    profileName: string;
    host: string;
    tlsFingerprint: string;
    token: string;
    scopes: string[];
    expiresAt: string;
  }) => Promise<void>;
}) {
  const navigate = useNavigate();
  const [clientName, setClientName] = useState(defaultClientName);
  const [payloadText, setPayloadText] = useState('');
  const [browserHost, setBrowserHost] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [scannerOpen, setScannerOpen] = useState(false);
  const [parsedPayload, setParsedPayload] = useState<PairPayload | null>(null);

  useEffect(() => {
    try {
      if (!payloadText.trim()) {
        setParsedPayload(null);
        setBrowserHost('');
        return;
      }

      const payload = parsePairPayload(payloadText);
      setParsedPayload(payload);
      setBrowserHost((current) => {
        if (current.trim()) {
          return current;
        }
        return deriveBrowserPairTarget(payload.h).suggestedBrowserHost;
      });
    } catch {
      setParsedPayload(null);
    }
  }, [payloadText]);

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError(null);

    try {
      const payload = parsePairPayload(payloadText);
      const fallbackHost = deriveBrowserPairTarget(payload.h).suggestedBrowserHost;
      const targetHost = requireBrowserSafeHost(browserHost.trim() || fallbackHost);
      const apiBaseUrl = deriveApiBaseUrl(targetHost);
      const pairResult = await pairClient(apiBaseUrl, {
        code: payload.c,
        clientName,
        clientType: 'web',
      });

      await props.onSaveProfile({
        profileName: pairResult.server_name,
        host: targetHost,
        tlsFingerprint: payload.fp,
        token: pairResult.token,
        scopes: pairResult.scopes,
        expiresAt: pairResult.expires_at,
      });

      navigate('/app');
    } catch (submitError) {
      setError(submitError instanceof Error ? submitError.message : 'pairing failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex min-h-dvh flex-col">
      <PageToolbar title="pair with qr" />

      <div className="mx-auto w-full max-w-lg flex-1 px-4 py-6">
        <p className="text-sm text-slate-500">
          scan or paste the qr payload from <code className="text-xs">imsg-bridge-cli pair</code>.
        </p>

        {scannerOpen ? (
          <div className="mt-4">
            <QrScannerPanel onPayload={setPayloadText} />
          </div>
        ) : null}

        <form className="mt-5 space-y-4" onSubmit={handleSubmit}>
          <label className="block">
            <span className="mb-1.5 block text-xs font-medium text-slate-700">client name</span>
            <input
              className="field"
              value={clientName}
              onChange={(event) => setClientName(event.target.value)}
              placeholder="browser on this device"
            />
          </label>

          <label className="block">
            <span className="mb-1.5 block text-xs font-medium text-slate-700">qr payload</span>
            <textarea
              className="field min-h-28"
              value={payloadText}
              onChange={(event) => setPayloadText(event.target.value)}
              placeholder='{"h":"100.64.0.3:8443","fp":"SHA256:...","c":"ABC123","v":1}'
            />
          </label>

          <label className="block">
            <span className="mb-1.5 block text-xs font-medium text-slate-700">browser host</span>
            <input
              className="field"
              value={browserHost}
              onChange={(event) => setBrowserHost(event.target.value)}
              placeholder="bridge-name.your-tailnet.ts.net"
            />
            <p className="mt-1.5 text-xs text-slate-400">
              use a browser-trusted https hostname here, usually your <code>*.ts.net</code> serve host.
            </p>
          </label>

          <div className="flex gap-3">
            <button className="button-primary" disabled={busy} type="submit">
              {busy ? 'pairing…' : 'pair'}
            </button>
            <button
              className="button-secondary"
              onClick={() => setScannerOpen((current) => !current)}
              type="button"
            >
              {scannerOpen ? 'close camera' : 'scan qr'}
            </button>
          </div>

          {parsedPayload ? (
            <div className="rounded-xl bg-slate-50 px-3.5 py-3 text-xs text-slate-600">
              <p className="font-medium text-slate-900">payload detected</p>
              <p className="mt-1 break-all">
                host: <code>{parsedPayload.h}</code>
              </p>
              <p className="mt-0.5 break-all">
                target: <code>{browserHost || deriveBrowserPairTarget(parsedPayload.h).suggestedBrowserHost || 'enter your browser-safe host'}</code>
              </p>
            </div>
          ) : null}

          {error ? (
            <p className="rounded-xl bg-rose-50 px-3.5 py-3 text-sm text-rose-700">{error}</p>
          ) : null}
        </form>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// session page
// ---------------------------------------------------------------------------

function SessionPage(props: {
  onSaveProfile: (input: {
    profileName: string;
    host: string;
    tlsFingerprint: string;
    token: string;
    scopes: string[];
    expiresAt: string;
  }) => Promise<void>;
}) {
  const navigate = useNavigate();
  const [clientName, setClientName] = useState(defaultClientName);
  const [host, setHost] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [session, setSession] = useState<CreateSessionResponse | null>(null);

  useEffect(() => {
    if (!session) {
      return;
    }

    let cancelled = false;
    const apiBaseUrl = deriveApiBaseUrl(requireBrowserSafeHost(host));
    const timer = window.setInterval(() => {
      pollSession(apiBaseUrl, session.session_id)
        .then(async (result) => {
          if (cancelled) {
            return;
          }

          if (result.status === 'approved') {
            await props.onSaveProfile({
              profileName: host,
              host,
              tlsFingerprint: '',
              token: result.token,
              scopes: result.scopes,
              expiresAt: result.expires_at,
            });
            window.clearInterval(timer);
            navigate('/app');
          }

          if (result.status === 'expired') {
            setError('session expired before approval completed');
            setSession(null);
            window.clearInterval(timer);
          }
        })
        .catch((pollError) => {
          if (!cancelled) {
            setError(pollError instanceof Error ? pollError.message : 'session polling failed');
          }
        });
    }, 3000);

    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [host, navigate, props, session]);

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError(null);

    try {
      const apiBaseUrl = deriveApiBaseUrl(requireBrowserSafeHost(host));
      const created = await createSession(apiBaseUrl, {
        clientName,
        clientType: 'web',
      });
      setSession(created);
    } catch (submitError) {
      setError(submitError instanceof Error ? submitError.message : 'session request failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex min-h-dvh flex-col">
      <PageToolbar title="approval code" />

      <div className="mx-auto w-full max-w-lg flex-1 px-4 py-6">
        <p className="text-sm text-slate-500">
          link this browser via an already-paired device.
        </p>

        <form className="mt-5 space-y-4" onSubmit={handleSubmit}>
          <label className="block">
            <span className="mb-1.5 block text-xs font-medium text-slate-700">bridge host</span>
            <input
              className="field"
              value={host}
              onChange={(event) => setHost(event.target.value)}
              placeholder="bridge-name.your-tailnet.ts.net"
            />
            <p className="mt-1.5 text-xs text-slate-400">
              use the browser-safe https hostname, not the direct bridge ip or <code>:8443</code>.
            </p>
          </label>
          <label className="block">
            <span className="mb-1.5 block text-xs font-medium text-slate-700">client name</span>
            <input
              className="field"
              value={clientName}
              onChange={(event) => setClientName(event.target.value)}
            />
          </label>

          <button className="button-primary" disabled={busy} type="submit">
            {busy ? 'creating…' : 'start session'}
          </button>

          {error ? (
            <p className="rounded-xl bg-rose-50 px-3.5 py-3 text-sm text-rose-700">{error}</p>
          ) : null}
        </form>

        {session ? (
          <div className="mt-6 rounded-xl border border-slate-200 bg-white p-5 text-center">
            <p className="text-xs font-medium text-slate-500">enter this code on your paired device</p>
            <p className="mt-3 text-4xl font-bold tracking-[0.25em] text-signal">
              {session.code}
            </p>
            <p className="mt-3 text-xs text-slate-400">
              session {session.session_id} · expires {session.expires_at}
            </p>
          </div>
        ) : null}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// settings page
// ---------------------------------------------------------------------------

function SettingsPage({ profile }: { profile: StoredServerProfile }) {
  return (
    <div className="flex min-h-dvh flex-col">
      <PageToolbar title="settings" />

      <div className="mx-auto w-full max-w-lg flex-1 px-4 py-6">
        <div className="divide-y divide-slate-100">
          <SettingsRow label="bridge" value={profile.name} />
          <SettingsRow label="api host" value={profile.apiBaseUrl} />
          <SettingsRow label="websocket" value={`${profile.wsBaseUrl}/v1/events`} />
          <SettingsRow
            label="tls fingerprint"
            value={profile.tlsFingerprint || 'not captured'}
          />
          <SettingsRow label="scopes" value={profile.scopes.join(', ')} />
          <SettingsRow label="token expiry" value={profile.expiresAt} />
        </div>

        <div className="mt-6 flex gap-3">
          <Link className="button-secondary" to="/pair">pair new bridge</Link>
          <Link className="button-secondary" to="/session">approval code</Link>
        </div>

        <div className="mt-4">
          <Link className="button-primary w-full justify-center" to="/app">
            back to messages
          </Link>
        </div>
      </div>
    </div>
  );
}

function SettingsRow(props: { label: string; value: string }) {
  return (
    <div className="py-3">
      <p className="text-xs font-medium text-slate-500">{props.label}</p>
      <p className="mt-1 break-all text-sm text-slate-900">{props.value}</p>
    </div>
  );
}

// ---------------------------------------------------------------------------
// app shell — main messaging view
// ---------------------------------------------------------------------------

function AppShell({ profile }: { profile: StoredServerProfile }) {
  const activeThreadRequest = useRef(0);
  const chatsRef = useRef<Chat[]>([]);
  const messagesRef = useRef<Message[]>([]);
  const activeChatIdRef = useRef<number | null>(null);
  const [token, setToken] = useState<string | null>(null);
  const [serverInfo, setServerInfo] = useState<ServerInfo | null>(null);
  const [chats, setChats] = useState<Chat[]>([]);
  const [messages, setMessages] = useState<Message[]>([]);
  const [activeChatId, setActiveChatId] = useState<number | null>(null);
  const [status, setStatus] = useState<
    'idle' | 'loading' | 'refreshing' | 'ready' | 'error'
  >('idle');
  const [threadStatus, setThreadStatus] = useState<
    'idle' | 'loading' | 'refreshing' | 'ready' | 'error'
  >('idle');
  const [eventsStatus, setEventsStatus] = useState<'idle' | 'connecting' | 'live' | 'error'>(
    'idle',
  );
  const [error, setError] = useState<string | null>(null);
  const [threadError, setThreadError] = useState<string | null>(null);
  const [eventsError, setEventsError] = useState<string | null>(null);
  const [canLoadOlder, setCanLoadOlder] = useState(false);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const [mobileShowThread, setMobileShowThread] = useState(false);

  const activeChat = useMemo(
    () => chats.find((chat) => chat.id === activeChatId) ?? null,
    [activeChatId, chats],
  );
  const canConnectEvents = Boolean(token) && (status === 'ready' || status === 'refreshing');

  useEffect(() => {
    chatsRef.current = chats;
  }, [chats]);

  useEffect(() => {
    messagesRef.current = messages;
  }, [messages]);

  useEffect(() => {
    activeChatIdRef.current = activeChatId;
  }, [activeChatId]);

  // websocket
  useEffect(() => {
    if (!token || !canConnectEvents) {
      setEventsStatus('idle');
      setEventsError(null);
      return;
    }

    let cancelled = false;
    let socket: WebSocket | null = null;
    let reconnectTimer = 0;
    let reconnectDelay = 1000;
    let lastCloseMessage: string | null = null;

    const connect = () => {
      if (cancelled) {
        return;
      }

      setEventsStatus('connecting');
      setEventsError(null);

      const url = new URL(`${profile.wsBaseUrl}/v1/events`);
      url.searchParams.set('token', token);

      socket = new WebSocket(url.toString());

      socket.onopen = () => {
        if (cancelled) {
          return;
        }
        reconnectDelay = 1000;
        lastCloseMessage = null;
        setEventsStatus('live');
        setEventsError(null);
      };

      socket.onmessage = (event) => {
        if (cancelled) {
          return;
        }

        try {
          const payload = JSON.parse(event.data) as BridgeEvent;
          if (payload.type === 'heartbeat') {
            setEventsStatus('live');
            return;
          }

          const message = payload.data;
          setEventsStatus('live');
          void applyIncomingMessage(message);
        } catch (wsError) {
          console.error('failed to parse bridge event', wsError);
        }
      };

      socket.onerror = () => {
        if (cancelled) {
          return;
        }
        lastCloseMessage = 'live updates could not connect. retrying…';
        setEventsStatus('error');
        setEventsError(lastCloseMessage);
      };

      socket.onclose = (event) => {
        if (cancelled) {
          return;
        }

        const closeMessage = describeEventStreamClose(event, lastCloseMessage);
        const nextDelay = reconnectDelay;

        setEventsStatus('error');
        setEventsError(closeMessage);

        reconnectTimer = window.setTimeout(() => {
          if (cancelled) {
            return;
          }
          reconnectDelay = Math.min(reconnectDelay * 2, 15000);
          setEventsStatus('connecting');
          setEventsError(null);
          connect();
        }, nextDelay);
      };
    };

    connect();

    return () => {
      cancelled = true;
      window.clearTimeout(reconnectTimer);
      socket?.close();
    };
  }, [canConnectEvents, profile.wsBaseUrl, token]);

  // initial load
  useEffect(() => {
    let cancelled = false;

    async function loadShell() {
      setStatus('loading');
      setThreadStatus('idle');
      setError(null);
      setThreadError(null);

      try {
        const decryptedToken = await decryptString(profile.token);
        setToken(decryptedToken);

        const [cachedChats, savedActiveChatId] = await Promise.all([
          getCachedChats(profile.id),
          getActiveChatId(profile.id),
        ]);

        let bootChatId: number | null = null;
        const sortedCachedChats = sortChats(cachedChats);
        if (!cancelled && sortedCachedChats.length > 0) {
          const initialCachedChat =
            sortedCachedChats.find((chat) => chat.id === savedActiveChatId) ?? sortedCachedChats[0];
          bootChatId = initialCachedChat.id;
          setChats(sortedCachedChats);
          setActiveChatId(initialCachedChat.id);
          setStatus('refreshing');
          void loadMessagesForChat(initialCachedChat.id, decryptedToken, false);
        }

        const [server, loadedChats] = await Promise.all([
          fetchServerInfo(profile.apiBaseUrl, decryptedToken),
          listChats(profile.apiBaseUrl, decryptedToken),
        ]);

        if (!cancelled) {
          setServerInfo(server);
          const sortedChats = sortChats(mergeLoadedChats(sortedCachedChats, loadedChats));
          setChats(sortedChats);
          void saveChats(profile.id, sortedChats).catch((cacheError) => {
            console.error('failed to cache chats', cacheError);
          });
          setStatus('ready');

          if (sortedChats.length === 0) {
            setActiveChatId(null);
            setMessages([]);
            setCanLoadOlder(false);
            setThreadStatus('ready');
            return;
          }

          const initialChat =
            sortedChats.find((chat) => chat.id === (bootChatId ?? savedActiveChatId)) ?? sortedChats[0];

          setActiveChatId(initialChat.id);
          if (bootChatId === null || bootChatId !== initialChat.id) {
            void loadMessagesForChat(initialChat.id, decryptedToken, false);
          }
        }
      } catch (loadError) {
        if (!cancelled) {
          setStatus(chatsRef.current.length > 0 ? 'ready' : 'error');
          if (messagesRef.current.length === 0) {
            setThreadStatus('error');
          }
          setError(
            loadError instanceof Error
              ? loadError.message
              : 'could not reach the selected bridge',
          );
        }
      }
    }

    void loadShell();

    return () => {
      cancelled = true;
    };
  }, [profile]);

  async function loadMessagesForChat(
    chatId: number,
    activeToken = token,
    persistSelection = true,
  ) {
    if (!activeToken) {
      setThreadStatus('error');
      setThreadError('saved token is not available yet');
      return;
    }

    const requestId = ++activeThreadRequest.current;
    setThreadError(null);
    setActiveChatId(chatId);

    if (persistSelection) {
      void setActiveChat(profile.id, chatId).catch((persistError) => {
        console.error('failed to persist active chat', persistError);
      });
    }

    const cachedMessages = await getCachedMessages(profile.id, chatId);
    if (requestId !== activeThreadRequest.current) {
      return;
    }

    if (cachedMessages.length > 0) {
      const sortedCachedMessages = sortMessages(cachedMessages);
      setMessages(sortedCachedMessages);
      const nextChats = updateChatPreview(
        chatsRef.current,
        chatId,
        latestMessagePreview(sortedCachedMessages),
      );
      setChats(nextChats);
      void saveChats(profile.id, nextChats).catch((cacheError) => {
        console.error('failed to backfill cached chat previews', cacheError);
      });
      setCanLoadOlder(cachedMessages.length >= recentMessageLimit);
      setThreadStatus('refreshing');
    } else {
      setMessages([]);
      setCanLoadOlder(false);
      setThreadStatus('loading');
    }
    setLoadingOlder(false);

    try {
      const loadedMessages = await listMessages(profile.apiBaseUrl, activeToken, chatId, {
        attachments: false,
        limit: recentMessageLimit,
      });
      if (requestId !== activeThreadRequest.current) {
        return;
      }

      const sortedMessages = sortMessages(loadedMessages);
      setMessages(sortedMessages);
      setCanLoadOlder(loadedMessages.length >= recentMessageLimit);
      setThreadStatus('ready');
      const nextChats = updateChatPreview(chatsRef.current, chatId, latestMessagePreview(sortedMessages));
      setChats(nextChats);
      void saveChats(profile.id, nextChats).catch((cacheError) => {
        console.error('failed to cache chat previews', cacheError);
      });
      void saveMessages(profile.id, chatId, sortedMessages).catch((cacheError) => {
        console.error('failed to cache messages', cacheError);
      });
    } catch (loadError) {
      if (requestId !== activeThreadRequest.current) {
        return;
      }

      if (cachedMessages.length > 0) {
        setThreadStatus('ready');
        setThreadError(
          loadError instanceof Error
            ? `${loadError.message}. showing cached history.`
            : 'could not refresh this conversation. showing cached history.',
        );
        return;
      }

      setThreadStatus('error');
      setThreadError(
        loadError instanceof Error ? loadError.message : 'could not load this conversation',
      );
    }
  }

  async function loadOlderMessages() {
    if (!activeChatId || !token || loadingOlder || messages.length === 0) {
      return;
    }

    const oldestTimestamp = messages.find((message) => message.created_at)?.created_at;
    if (!oldestTimestamp) {
      setCanLoadOlder(false);
      return;
    }

    setLoadingOlder(true);
    setThreadError(null);

    try {
      const olderMessages = await listMessages(profile.apiBaseUrl, token, activeChatId, {
        attachments: false,
        limit: olderMessagePageSize,
        before: oldestTimestamp,
      });
      const mergedMessages = mergeMessages(messages, olderMessages);
      setMessages(mergedMessages);
      setCanLoadOlder(olderMessages.length >= olderMessagePageSize);
      const nextChats = updateChatPreview(chatsRef.current, activeChatId, latestMessagePreview(mergedMessages));
      setChats(nextChats);
      void saveChats(profile.id, nextChats).catch((cacheError) => {
        console.error('failed to cache chat previews after older load', cacheError);
      });
      void saveMessages(profile.id, activeChatId, mergedMessages).catch((cacheError) => {
        console.error('failed to cache merged messages', cacheError);
      });
    } catch (loadError) {
      setThreadError(
        loadError instanceof Error
          ? `could not load older messages: ${loadError.message}`
          : 'could not load older messages',
      );
    } finally {
      setLoadingOlder(false);
    }
  }

  async function applyIncomingMessage(message: Message) {
    const nextChats = bumpChatActivity(chatsRef.current, message);
    setChats(nextChats);
    void saveChats(profile.id, nextChats).catch((cacheError) => {
      console.error('failed to refresh cached chats from event stream', cacheError);
    });

    if (message.chat_id !== activeChatIdRef.current) {
      return;
    }

    const nextMessages = mergeMessages(messagesRef.current, [message]);
    setMessages(nextMessages);
    void saveMessages(profile.id, message.chat_id, nextMessages).catch((cacheError) => {
      console.error('failed to refresh cached messages from event stream', cacheError);
    });
  }

  function selectChat(chatId: number) {
    if (chatId === activeChatId) {
      return;
    }
    void loadMessagesForChat(chatId);
    setMobileShowThread(true);
  }

  return (
    <div className="flex h-dvh flex-col lg:flex-row">
      {/* sidebar — hidden on mobile when thread is shown */}
      <div
        className={`flex flex-col border-r border-slate-100 lg:w-80 lg:shrink-0 ${
          mobileShowThread ? 'hidden lg:flex' : 'flex'
        }`}
      >
        <div className="toolbar">
          <h1 className="text-sm font-semibold text-slate-900">messages</h1>
          <div className="ml-auto flex items-center gap-2">
            <StatusDot status={eventsStatus} detail={eventsError} />
            <Link to="/settings" className="text-slate-400 hover:text-slate-600">
              <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="h-5 w-5">
                <path fillRule="evenodd" d="M7.84 1.804A1 1 0 018.82 1h2.36a1 1 0 01.98.804l.331 1.652a6.993 6.993 0 011.929 1.115l1.598-.54a1 1 0 011.186.447l1.18 2.044a1 1 0 01-.205 1.251l-1.267 1.113a7.047 7.047 0 010 2.228l1.267 1.113a1 1 0 01.206 1.25l-1.18 2.045a1 1 0 01-1.187.447l-1.598-.54a6.993 6.993 0 01-1.929 1.115l-.33 1.652a1 1 0 01-.98.804H8.82a1 1 0 01-.98-.804l-.331-1.652a6.993 6.993 0 01-1.929-1.115l-1.598.54a1 1 0 01-1.186-.447l-1.18-2.044a1 1 0 01.205-1.251l1.267-1.114a7.05 7.05 0 010-2.227L1.821 7.773a1 1 0 01-.206-1.25l1.18-2.045a1 1 0 011.187-.447l1.598.54A6.993 6.993 0 017.51 3.456l.33-1.652zM10 13a3 3 0 100-6 3 3 0 000 6z" clipRule="evenodd" />
              </svg>
            </Link>
          </div>
        </div>

        <ChatList
          activeChatId={activeChatId}
          chats={chats}
          disabled={status === 'loading' && chats.length === 0}
          status={status}
          onSelectChat={selectChat}
        />
        {eventsError ? (
          <div className="border-t border-slate-100 bg-amber-50 px-4 py-2 text-xs text-amber-700">
            {eventsError}
          </div>
        ) : null}
      </div>

      {/* thread — hidden on mobile when chat list is shown */}
      <div
        className={`flex min-w-0 flex-1 flex-col ${
          mobileShowThread ? 'flex' : 'hidden lg:flex'
        }`}
      >
        <ThreadView
          activeChat={activeChat}
          canLoadOlder={canLoadOlder}
          loadingOlder={loadingOlder}
          messages={messages}
          onLoadOlder={() => {
            void loadOlderMessages();
          }}
          shellStatus={status}
          onReload={() => {
            if (activeChatId !== null) {
              void loadMessagesForChat(activeChatId, token, false);
            }
          }}
          onBack={() => setMobileShowThread(false)}
          status={threadStatus}
          threadError={threadError}
          eventsError={eventsError}
          eventsStatus={eventsStatus}
        />
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// chat list
// ---------------------------------------------------------------------------

function ChatList(props: {
  chats: Chat[];
  activeChatId: number | null;
  disabled: boolean;
  status: 'idle' | 'loading' | 'refreshing' | 'ready' | 'error';
  onSelectChat: (chatId: number) => void;
}) {
  return (
    <div className="flex-1 overflow-y-auto">
      {props.status === 'loading' && props.chats.length === 0 ? (
        <div className="flex items-center justify-center py-12">
          <div className="text-center">
            <div className="mx-auto h-6 w-6 animate-spin rounded-full border-2 border-slate-200 border-t-signal" />
            <p className="mt-2 text-xs text-slate-400">loading chats…</p>
          </div>
        </div>
      ) : props.chats.length === 0 ? (
        <div className="px-4 py-8 text-center text-sm text-slate-400">
          no conversations yet.
        </div>
      ) : (
        <div className="py-1">
          {props.chats.map((chat) => {
            const active = chat.id === props.activeChatId;
            const initials = chatInitials(displayChatName(chat));
            return (
              <button
                key={chat.id}
                className={`chat-item ${active ? 'chat-item--active' : ''}`}
                disabled={props.disabled}
                onClick={() => props.onSelectChat(chat.id)}
                type="button"
              >
                <div className="avatar h-10 w-10">{initials}</div>
                <div className="min-w-0 flex-1">
                  <div className="flex items-baseline justify-between gap-2">
                    <p className="truncate text-sm font-medium text-slate-900">
                      {displayChatName(chat)}
                    </p>
                    <span className="shrink-0 text-[11px] text-slate-400">
                      {formatChatTimestamp(chat.last_message_at)}
                    </span>
                  </div>
                  <p className="mt-0.5 truncate text-xs text-slate-500">
                    {chat.preview_text || chat.identifier || chat.service || `chat ${chat.id}`}
                  </p>
                </div>
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// thread view
// ---------------------------------------------------------------------------

function ThreadView(props: {
  activeChat: Chat | null;
  canLoadOlder: boolean;
  loadingOlder: boolean;
  messages: Message[];
  onLoadOlder: () => void;
  shellStatus: 'idle' | 'loading' | 'refreshing' | 'ready' | 'error';
  status: 'idle' | 'loading' | 'refreshing' | 'ready' | 'error';
  threadError: string | null;
  onReload: () => void;
  onBack: () => void;
  eventsError: string | null;
  eventsStatus: 'idle' | 'connecting' | 'live' | 'error';
}) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const prevMessageCount = useRef(0);

  useEffect(() => {
    if (props.messages.length > prevMessageCount.current && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
    prevMessageCount.current = props.messages.length;
  }, [props.messages.length]);

  return (
    <>
      {/* thread toolbar */}
      <div className="toolbar">
        <button
          className="text-slate-400 hover:text-slate-600 lg:hidden"
          onClick={props.onBack}
          type="button"
        >
          <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="h-5 w-5">
            <path fillRule="evenodd" d="M17 10a.75.75 0 01-.75.75H5.612l4.158 3.96a.75.75 0 11-1.04 1.08l-5.5-5.25a.75.75 0 010-1.08l5.5-5.25a.75.75 0 111.04 1.08L5.612 9.25H16.25A.75.75 0 0117 10z" clipRule="evenodd" />
          </svg>
        </button>
        <p className="truncate text-sm font-semibold text-slate-900">
          {props.activeChat ? displayChatName(props.activeChat) : 'select a chat'}
        </p>
        <div className="ml-auto flex items-center gap-2">
          <StatusDot status={props.eventsStatus} detail={props.eventsError} />
          <button
            className="rounded-md p-1.5 text-slate-400 hover:bg-slate-50 hover:text-slate-600"
            disabled={!props.activeChat}
            onClick={props.onReload}
            type="button"
            title="reload"
          >
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="h-4 w-4">
              <path fillRule="evenodd" d="M15.312 11.424a5.5 5.5 0 01-9.201 2.466l-.312-.311h2.433a.75.75 0 000-1.5H4.598a.75.75 0 00-.75.75v3.634a.75.75 0 001.5 0v-2.033l.312.311a7 7 0 0011.712-3.138.75.75 0 00-1.449-.39zm-10.624-3.95a.75.75 0 00.174-1.048 5.5 5.5 0 019.201-2.466l.312.311H12.94a.75.75 0 000 1.5h3.634a.75.75 0 00.75-.75V1.387a.75.75 0 00-1.5 0v2.033l-.312-.311A7 7 0 003.8 6.247a.75.75 0 001.048.174l-.161.053z" clipRule="evenodd" />
            </svg>
          </button>
        </div>
      </div>

      {/* messages area */}
      <div
        ref={scrollRef}
        className="flex-1 overflow-y-auto bg-[linear-gradient(180deg,#f4f5f7_0%,#eef1f5_100%)] px-3 py-3 sm:px-4"
      >
        {props.activeChat && props.canLoadOlder ? (
          <div className="mb-4 flex justify-center">
            <button
              className="rounded-full border border-white/80 bg-white/90 px-3 py-1.5 text-[11px] font-medium text-slate-500 shadow-[0_6px_16px_rgba(15,23,42,0.08)] backdrop-blur hover:bg-white"
              disabled={props.loadingOlder}
              onClick={props.onLoadOlder}
              type="button"
            >
              {props.loadingOlder ? 'loading…' : 'load older'}
            </button>
          </div>
        ) : null}

        {props.status === 'idle' && !props.activeChat ? (
          <div className="flex h-full items-center justify-center text-sm text-slate-400">
            pick a conversation
          </div>
        ) : null}

        {props.status === 'loading' && props.messages.length === 0 ? (
          <div className="flex h-full items-center justify-center">
            <div className="text-center">
              <div className="mx-auto h-6 w-6 animate-spin rounded-full border-2 border-slate-200 border-t-signal" />
              <p className="mt-2 text-xs text-slate-400">loading messages…</p>
            </div>
          </div>
        ) : null}

        {props.status === 'error' && props.messages.length === 0 ? (
          <div className="rounded-lg bg-rose-50 px-3.5 py-3 text-sm text-rose-700">
            {props.threadError || 'failed to load thread'}
          </div>
        ) : null}

        {(props.status === 'ready' || props.status === 'refreshing') && props.messages.length === 0 ? (
          <div className="flex h-full items-center justify-center text-sm text-slate-400">
            no messages in this conversation yet.
          </div>
        ) : null}

        {props.threadError && props.messages.length > 0 ? (
          <div className="mb-3 rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-700">
            {props.threadError}
          </div>
        ) : null}

        <div className="space-y-2">
          {props.messages.map((message) => (
            <MessageBubble key={message.guid || String(message.id)} message={message} />
          ))}
        </div>
      </div>

      {/* footer */}
      <div className="border-t border-slate-100 px-4 py-2.5 text-center text-xs text-slate-400">
        read-only — send coming soon
      </div>
    </>
  );
}

// ---------------------------------------------------------------------------
// message bubble
// ---------------------------------------------------------------------------

function MessageBubble({ message }: { message: Message }) {
  const fromMe = message.is_from_me;
  const text = message.text?.trim();
  const attachments = message.attachments ?? [];
  const reactions = message.reactions ?? [];

  return (
    <div className={fromMe ? 'flex justify-end' : 'flex justify-start'}>
      <div className="max-w-[78%] sm:max-w-[72%]">
        {!fromMe ? (
          <p className="mb-1 px-1.5 text-[11px] font-medium text-slate-500">
            {message.sender || 'contact'}
          </p>
        ) : null}
        <div
          className={
            fromMe
              ? 'rounded-[1.4rem] rounded-br-md bg-signal px-3.5 py-2.5 text-white shadow-[0_10px_24px_rgba(10,132,255,0.26)]'
              : 'rounded-[1.4rem] rounded-bl-md border border-slate-300/70 bg-slate-200 px-3.5 py-2.5 text-slate-950 shadow-[0_8px_20px_rgba(15,23,42,0.08)]'
          }
        >
          {text ? (
            <p className="whitespace-pre-wrap text-[15px] leading-[1.35] tracking-[-0.01em]">
              {text}
            </p>
          ) : null}

          {!text && attachments.length > 0 ? (
            <p className={fromMe ? 'text-sm text-white/80' : 'text-sm text-slate-700'}>
              attachment-only message
            </p>
          ) : null}

          {attachments.length > 0 ? (
            <div className="mt-2 flex flex-wrap gap-1.5">
              {attachments.map((attachment, index) => (
                <span
                  key={attachment.id || `${message.id}-${index}`}
                  className={
                    fromMe
                      ? 'rounded-full bg-white/16 px-2.5 py-1 text-[11px] font-medium text-white'
                      : 'rounded-full bg-white/75 px-2.5 py-1 text-[11px] font-medium text-slate-700 ring-1 ring-slate-300/70'
                  }
                >
                  {attachment.filename || 'attachment'}
                  {attachment.size_bytes ? ` · ${formatBytes(attachment.size_bytes)}` : ''}
                </span>
              ))}
            </div>
          ) : null}
        </div>

        {reactions.length > 0 ? (
          <p className="mt-1 px-1.5 text-[11px] text-slate-400">
            {reactions
              .map((reaction) => reaction.emoji || reaction.type || 'reaction')
              .join(' ')}
          </p>
        ) : null}

        <p
          className={`mt-1 px-1.5 text-[10px] ${fromMe ? 'text-right text-slate-400' : 'text-slate-500'}`}
        >
          {formatMessageTimestamp(message.created_at)}
        </p>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// status dot
// ---------------------------------------------------------------------------

function StatusDot({
  status,
  detail,
}: {
  status: 'idle' | 'loading' | 'refreshing' | 'ready' | 'error' | 'connecting' | 'live';
  detail?: string | null;
}) {
  const dotColor: Record<typeof status, string> = {
    idle: 'bg-slate-300',
    loading: 'bg-amber-400',
    refreshing: 'bg-sky-400',
    ready: 'bg-signal',
    connecting: 'bg-amber-400',
    live: 'bg-signal',
    error: 'bg-rose-500',
  };
  const label: Record<typeof status, string> = {
    idle: 'idle',
    loading: 'loading',
    refreshing: 'refreshing',
    ready: 'ready',
    connecting: 'connecting',
    live: 'connected',
    error: 'updates offline',
  };

  return (
    <span className="flex items-center gap-1.5" title={detail || label[status]}>
      <span className={`inline-block h-2 w-2 rounded-full ${dotColor[status]}`} />
      <span className="text-[11px] text-slate-500">{label[status]}</span>
    </span>
  );
}

function describeEventStreamClose(event: CloseEvent, fallback: string | null): string {
  const reason = event.reason.trim();
  if (reason) {
    return `live updates closed: ${reason}`;
  }

  switch (event.code) {
    case 1000:
      return fallback ?? 'live updates disconnected. retrying…';
    case 1006:
      return fallback ?? 'live updates could not connect. retrying…';
    case 1008:
      return 'live updates were rejected by the server. check the browser host and token.';
    case 1013:
      return 'live updates are busy right now. retrying…';
    default:
      return fallback ?? `live updates disconnected (code ${event.code}). retrying…`;
  }
}

// ---------------------------------------------------------------------------
// qr scanner
// ---------------------------------------------------------------------------

function QrScannerPanel({ onPayload }: { onPayload: (value: string) => void }) {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const [status, setStatus] = useState('starting camera…');
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!window.BarcodeDetector) {
      setError('barcode detection not available in this browser — paste the payload manually');
      return;
    }

    let cancelled = false;
    let stream: MediaStream | null = null;
    let frameHandle = 0;
    const detector = new window.BarcodeDetector({ formats: ['qr_code'] });

    async function start() {
      try {
        stream = await navigator.mediaDevices.getUserMedia({
          video: { facingMode: 'environment' },
        });

        if (!videoRef.current) {
          return;
        }

        videoRef.current.srcObject = stream;
        await videoRef.current.play();
        setStatus('scanning…');

        const scan = async () => {
          if (cancelled || !videoRef.current) {
            return;
          }

          try {
            const results = await detector.detect(videoRef.current);
            const payload = results.find((item) => item.rawValue)?.rawValue;
            if (payload) {
              onPayload(payload);
              setStatus('captured');
              stream?.getTracks().forEach((track) => track.stop());
              return;
            }
          } catch (scanError) {
            setError(
              scanError instanceof Error ? scanError.message : 'camera scan failed',
            );
          }

          frameHandle = window.requestAnimationFrame(scan);
        };

        frameHandle = window.requestAnimationFrame(scan);
      } catch (cameraError) {
        setError(
          cameraError instanceof Error
            ? cameraError.message
            : 'camera access failed',
        );
      }
    }

    void start();

    return () => {
      cancelled = true;
      window.cancelAnimationFrame(frameHandle);
      stream?.getTracks().forEach((track) => track.stop());
    };
  }, [onPayload]);

  return (
    <div>
      <div className="overflow-hidden rounded-xl border border-slate-200 bg-black">
        <video className="aspect-square w-full object-cover" muted playsInline ref={videoRef} />
      </div>
      <p className="mt-2 text-xs text-slate-500">{status}</p>
      {error ? (
        <p className="mt-2 rounded-lg bg-rose-50 px-3 py-2 text-xs text-rose-700">{error}</p>
      ) : null}
    </div>
  );
}

// ---------------------------------------------------------------------------
// helpers — all business logic unchanged
// ---------------------------------------------------------------------------

function chatInitials(name: string): string {
  const parts = name.trim().split(/\s+/);
  if (parts.length >= 2) {
    return (parts[0][0] + parts[1][0]).toUpperCase();
  }
  return name.slice(0, 2).toUpperCase();
}

function sortChats(chats: Chat[]): Chat[] {
  return [...chats].sort((left, right) => {
    const leftAt = left.last_message_at ? Date.parse(left.last_message_at) : 0;
    const rightAt = right.last_message_at ? Date.parse(right.last_message_at) : 0;
    if (leftAt !== rightAt) {
      return rightAt - leftAt;
    }
    return right.id - left.id;
  });
}

function sortMessages(messages: Message[]): Message[] {
  return [...messages].sort((left, right) => {
    const leftAt = left.created_at ? Date.parse(left.created_at) : 0;
    const rightAt = right.created_at ? Date.parse(right.created_at) : 0;
    if (leftAt !== rightAt) {
      return leftAt - rightAt;
    }
    return left.id - right.id;
  });
}

function displayChatName(chat: Chat): string {
  return chat.name?.trim() || chat.identifier?.trim() || `chat ${chat.id}`;
}

function formatChatTimestamp(value?: string): string {
  if (!value) {
    return '';
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return '';
  }

  const now = new Date();
  const sameDay = now.toDateString() === date.toDateString();
  if (sameDay) {
    return new Intl.DateTimeFormat(undefined, {
      hour: 'numeric',
      minute: '2-digit',
    }).format(date);
  }

  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
  }).format(date);
}

function formatMessageTimestamp(value?: string): string {
  if (!value) {
    return '';
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return '';
  }

  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  }).format(date);
}

function formatBytes(value: number): string {
  if (value < 1024) {
    return `${value} b`;
  }

  if (value < 1024 * 1024) {
    return `${(value / 1024).toFixed(1)} kb`;
  }

  return `${(value / (1024 * 1024)).toFixed(1)} mb`;
}

function mergeMessages(current: Message[], older: Message[]): Message[] {
  const byKey = new Map<string, Message>();

  for (const message of [...current, ...older]) {
    byKey.set(messageKey(message), message);
  }

  return sortMessages(Array.from(byKey.values()));
}

function messageKey(message: Message): string {
  return message.guid?.trim() || String(message.id);
}

function mergeLoadedChats(cached: Chat[], loaded: Chat[]): Chat[] {
  const previews = new Map<number, string>();
  for (const chat of cached) {
    if (chat.preview_text) {
      previews.set(chat.id, chat.preview_text);
    }
  }

  return loaded.map((chat) => ({
    ...chat,
    preview_text: previews.get(chat.id) || chat.preview_text,
  }));
}

function updateChatPreview(chats: Chat[], chatId: number, previewText: string): Chat[] {
  if (!previewText) {
    return chats;
  }

  let changed = false;
  const nextChats = chats.map((chat) => {
    if (chat.id !== chatId) {
      return chat;
    }
    if (chat.preview_text === previewText) {
      return chat;
    }
    changed = true;
    return { ...chat, preview_text: previewText };
  });

  return changed ? nextChats : chats;
}

function latestMessagePreview(messages: Message[]): string {
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    const preview = summarizeMessage(messages[index]);
    if (preview) {
      return preview;
    }
  }

  return '';
}

function summarizeMessage(message: Message): string {
  const text = message.text?.trim();
  if (text) {
    return text.replace(/\s+/g, ' ').slice(0, 140);
  }

  const attachmentCount = message.attachments?.length ?? 0;
  if (attachmentCount > 0) {
    if (attachmentCount === 1) {
      return message.is_from_me ? 'you sent an attachment' : 'attachment';
    }
    return message.is_from_me ? `you sent ${attachmentCount} attachments` : `${attachmentCount} attachments`;
  }

  const reactionCount = message.reactions?.length ?? 0;
  if (reactionCount > 0) {
    return reactionCount === 1 ? 'reaction update' : `${reactionCount} reactions`;
  }

  return '';
}

function bumpChatActivity(chats: Chat[], message: Message): Chat[] {
  const timestamp = message.created_at;
  const previewText = summarizeMessage(message);
  const nextChats = [...chats];
  const index = nextChats.findIndex((chat) => chat.id === message.chat_id);

  if (index >= 0) {
    nextChats[index] = {
      ...nextChats[index],
      last_message_at: timestamp || nextChats[index].last_message_at,
      preview_text: previewText || nextChats[index].preview_text,
    };
    return sortChats(nextChats);
  }

  nextChats.unshift({
    id: message.chat_id,
    identifier: message.sender || `chat ${message.chat_id}`,
    last_message_at: timestamp,
    preview_text: previewText,
  });

  return sortChats(nextChats);
}

async function loadState(): Promise<AppState> {
  const [profiles, activeProfileId] = await Promise.all([
    listProfiles(),
    getActiveProfileId(),
  ]);

  return {
    profiles,
    activeProfileId,
    loading: false,
  };
}

export default App;
