import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { Link, Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom';

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
      <div className="mx-auto flex min-h-screen max-w-5xl items-center justify-center px-6 py-16">
        <div className="panel w-full max-w-md p-8 text-center">
          <p className="pill">loading</p>
          <h1 className="mt-4 text-2xl font-semibold">bringing the shell back up</h1>
          <p className="mt-3 text-sm text-slate-600">
            restoring saved bridge profiles and local session state.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto min-h-screen max-w-7xl px-4 pb-24 pt-4 sm:px-6 sm:pb-8 lg:px-8">
      <TopBar activeProfile={activeProfile} />
      <main className="mt-4">
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
      </main>
      <BottomNav hasActiveProfile={Boolean(activeProfile)} />
    </div>
  );
}

function TopBar({ activeProfile }: { activeProfile: StoredServerProfile | null }) {
  const location = useLocation();

  return (
    <header className="panel sticky top-4 z-20 rounded-[32px] px-4 py-4 sm:px-6">
      <div className="flex items-center justify-between gap-4">
        <div className="min-w-0">
          <p className="pill">material shell</p>
          <div className="mt-3 flex items-center gap-3">
            <div className="flex h-11 w-11 items-center justify-center rounded-2xl bg-signal text-lg font-bold text-white shadow-sm">
              i
            </div>
            <div className="min-w-0">
              <h1 className="truncate text-xl font-semibold tracking-tight text-slate-900 sm:text-2xl">
                imsg-bridge
              </h1>
              <p className="truncate text-sm text-slate-500">
                private messaging over tailscale
              </p>
            </div>
          </div>
        </div>

        <div className="hidden items-center gap-2 sm:flex">
          <NavLink to="/" currentPath={location.pathname}>
            home
          </NavLink>
          <NavLink to="/app" currentPath={location.pathname}>
            chats
          </NavLink>
          <NavLink to="/settings" currentPath={location.pathname}>
            settings
          </NavLink>
        </div>
      </div>

      {activeProfile ? (
        <div className="mt-4 flex items-center justify-between rounded-[28px] bg-white/75 px-4 py-3 text-sm text-slate-600">
          <div className="min-w-0">
            <p className="text-xs uppercase tracking-[0.18em] text-slate-400">active bridge</p>
            <p className="mt-1 truncate font-medium text-slate-900">{activeProfile.name}</p>
          </div>
          <Link className="button-secondary hidden sm:inline-flex" to="/settings">
            manage
          </Link>
        </div>
      ) : null}
    </header>
  );
}

function NavLink(props: { to: string; currentPath: string; children: string }) {
  const active = props.currentPath === props.to;
  return (
    <Link
      to={props.to}
      className={
        active
          ? 'button-primary'
          : 'button-secondary'
      }
    >
      {props.children}
    </Link>
  );
}

function BottomNav({ hasActiveProfile }: { hasActiveProfile: boolean }) {
  const location = useLocation();

  return (
    <nav className="fixed bottom-4 left-4 right-4 z-30 sm:hidden">
      <div className="panel flex items-center justify-around rounded-[28px] px-3 py-2">
        <BottomNavItem currentPath={location.pathname} label="home" to="/" />
        <BottomNavItem currentPath={location.pathname} label="chats" to={hasActiveProfile ? '/app' : '/'} />
        <BottomNavItem currentPath={location.pathname} label="settings" to={hasActiveProfile ? '/settings' : '/'} />
      </div>
    </nav>
  );
}

function BottomNavItem(props: { currentPath: string; label: string; to: string }) {
  const active = props.currentPath === props.to;
  return (
    <Link
      className={
        active
          ? 'flex min-w-[88px] flex-col items-center rounded-2xl bg-glow px-4 py-2 text-xs font-semibold uppercase tracking-[0.16em] text-signal'
          : 'flex min-w-[88px] flex-col items-center rounded-2xl px-4 py-2 text-xs font-semibold uppercase tracking-[0.16em] text-slate-500'
      }
      to={props.to}
    >
      {props.label}
    </Link>
  );
}

function HomePage(props: {
  profiles: StoredServerProfile[];
  activeProfileId: string | null;
  onActivate: (id: string) => Promise<void>;
  onDelete: (id: string) => Promise<void>;
}) {
  return (
    <div className="grid gap-6 lg:grid-cols-[1.12fr_0.88fr]">
      <section className="panel overflow-hidden p-6 sm:p-8">
        <div className="rounded-[28px] bg-gradient-to-br from-signal/14 via-white to-sky-100 px-5 py-6 sm:px-6">
          <p className="pill">your bridges</p>
          <h2 className="mt-4 text-3xl font-semibold tracking-tight text-slate-900">
            pick the server this device should feel closest to
          </h2>
          <p className="mt-3 max-w-2xl text-sm leading-6 text-slate-600">
            this browser keeps bridge profiles, tokens, recent chats, and message cache locally
            so it can come back up like an app instead of a blank page.
          </p>
          <div className="mt-5 flex flex-wrap gap-3">
            <Link className="button-primary" to="/pair">
              pair with qr
            </Link>
            <Link className="button-secondary" to="/session">
              use approval code
            </Link>
          </div>
        </div>

        <div className="mt-6 space-y-4">
          {props.profiles.length === 0 ? (
            <div className="rounded-[28px] border border-dashed border-slate-300 bg-slate-50 p-6 text-sm text-slate-600">
              no bridge profiles yet. pair from a qr code or start a delegated session.
            </div>
          ) : (
            props.profiles.map((profile) => (
              <article
                key={profile.id}
                className={
                  props.activeProfileId === profile.id
                    ? 'rounded-[28px] border border-signal/20 bg-glow/70 p-5'
                    : 'rounded-[28px] border border-slate-200 bg-white p-5'
                }
              >
                <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                  <div>
                    <h3 className="text-lg font-semibold">{profile.name}</h3>
                    <p className="mt-2 break-all text-sm text-slate-500">{profile.apiBaseUrl}</p>
                    <p className="mt-3 text-xs uppercase tracking-[0.16em] text-slate-400">
                      {profile.scopes.join(' · ')}
                    </p>
                  </div>
                  <div className="flex gap-2">
                    <button
                      className="button-secondary"
                      onClick={() => void props.onDelete(profile.id)}
                      type="button"
                    >
                      remove
                    </button>
                    <button
                      className="button-primary"
                      onClick={() => void props.onActivate(profile.id)}
                      type="button"
                    >
                      {props.activeProfileId === profile.id ? 'active' : 'use this bridge'}
                    </button>
                  </div>
                </div>
              </article>
            ))
          )}
        </div>
      </section>

      <aside className="space-y-6">
        <InfoCard
          title="app model"
          body="cloudflare only serves the shell. every message request and live event stream still goes straight to the selected home bridge over tailscale."
        />
        <InfoCard
          title="why it feels app-like"
          body="profiles, encrypted tokens, chat cache, and thread history stay on this device so refreshes can rehydrate instead of starting from scratch."
        />
        <InfoCard
          title="settings moved out"
          body="bridge metadata, connection status, and pairing utilities belong in settings so the primary surfaces can stay focused on selecting a bridge and reading conversations."
        />
      </aside>
    </div>
  );
}

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
      const targetHost = browserHost.trim() || deriveBrowserPairTarget(payload.h).suggestedBrowserHost;
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
    <div className="grid gap-6 lg:grid-cols-[1.05fr_0.95fr]">
      <section className="panel p-6 sm:p-8">
        <p className="pill">direct pairing</p>
        <h2 className="mt-4 text-2xl font-semibold">pair this browser</h2>
        <p className="mt-3 text-sm text-slate-600">
          the qr comes from <code>imsg-bridge-cli pair</code>. the browser will pair
          directly with the bridge over tailscale.
        </p>

        <form className="mt-6 space-y-4" onSubmit={handleSubmit}>
          <label className="block">
            <span className="mb-2 block text-sm font-medium text-slate-700">client name</span>
            <input
              className="field"
              value={clientName}
              onChange={(event) => setClientName(event.target.value)}
              placeholder="browser on this device"
            />
          </label>

          <label className="block">
            <span className="mb-2 block text-sm font-medium text-slate-700">qr payload</span>
            <textarea
              className="field min-h-40"
              value={payloadText}
              onChange={(event) => setPayloadText(event.target.value)}
              placeholder='{"h":"100.64.0.3:8443","fp":"SHA256:...","c":"ABC123","v":1}'
            />
          </label>

          <label className="block">
            <span className="mb-2 block text-sm font-medium text-slate-700">
              browser host
            </span>
            <input
              className="field"
              value={browserHost}
              onChange={(event) => setBrowserHost(event.target.value)}
              placeholder="bridge-name.your-tailnet.ts.net"
            />
            <p className="mt-2 text-xs leading-5 text-slate-500">
              for the web shell, prefer the private <code>*.ts.net</code> serve host.
              raw <code>100.x</code> bridge hosts often fail in browsers because the
              browser cannot trust the bridge’s self-signed cert directly.
            </p>
          </label>

          <div className="flex flex-wrap gap-3">
            <button className="button-primary" disabled={busy} type="submit">
              {busy ? 'pairing…' : 'pair this browser'}
            </button>
            <button
              className="button-secondary"
              onClick={() => setScannerOpen((current) => !current)}
              type="button"
            >
              {scannerOpen ? 'close camera scan' : 'scan with camera'}
            </button>
          </div>

          {parsedPayload ? (
            <div className="rounded-2xl bg-slate-50 px-4 py-3 text-sm text-slate-600">
              <p className="font-medium text-slate-900">pairing payload detected</p>
              <p className="mt-2 break-all">
                bridge host in qr: <code>{parsedPayload.h}</code>
              </p>
              <p className="mt-1 break-all">
                browser target: <code>{browserHost || deriveBrowserPairTarget(parsedPayload.h).suggestedBrowserHost}</code>
              </p>
            </div>
          ) : null}

          {error ? (
            <p className="rounded-2xl bg-rose-50 px-4 py-3 text-sm text-rose-700">
              {error}
            </p>
          ) : null}
        </form>
      </section>

      <section className="space-y-6">
        {scannerOpen ? (
          <QrScannerPanel onPayload={setPayloadText} />
        ) : (
          <InfoCard
            title="camera scanning"
            body="if barcode detection is available in this browser, the shell can fill the payload for you. otherwise paste the qr payload manually."
          />
        )}
        <InfoCard
          title="what gets stored"
          body="the shell saves the selected browser host, websocket base, tls fingerprint, encrypted token, expiry metadata, and local shell state."
        />
      </section>
    </div>
  );
}

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
    const apiBaseUrl = deriveApiBaseUrl(host);
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
      const apiBaseUrl = deriveApiBaseUrl(host);
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
    <div className="grid gap-6 lg:grid-cols-[1.05fr_0.95fr]">
      <section className="panel p-6 sm:p-8">
        <p className="pill">delegated session</p>
        <h2 className="mt-4 text-2xl font-semibold">approve this browser from another device</h2>
        <p className="mt-3 text-sm text-slate-600">
          use this when a phone or another client already has access and you want to
          approve a fresh browser session.
        </p>

        <form className="mt-6 space-y-4" onSubmit={handleSubmit}>
          <label className="block">
            <span className="mb-2 block text-sm font-medium text-slate-700">bridge host</span>
            <input
              className="field"
              value={host}
              onChange={(event) => setHost(event.target.value)}
              placeholder="bridge-name.your-tailnet.ts.net"
            />
          </label>
          <label className="block">
            <span className="mb-2 block text-sm font-medium text-slate-700">client name</span>
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
            <p className="rounded-2xl bg-rose-50 px-4 py-3 text-sm text-rose-700">
              {error}
            </p>
          ) : null}
        </form>
      </section>

      <section className="space-y-6">
        <div className="panel p-6 sm:p-8">
          <p className="pill">approval state</p>
          {session ? (
            <>
              <h3 className="mt-4 text-3xl font-extrabold tracking-[0.22em] text-signal">
                {session.code}
              </h3>
              <p className="mt-3 text-sm text-slate-600">
                enter this code in an already-paired client. this page polls the bridge
                directly until approval lands.
              </p>
              <dl className="mt-6 space-y-3 text-sm text-slate-600">
                <div>
                  <dt className="font-medium text-slate-900">session id</dt>
                  <dd className="break-all">{session.session_id}</dd>
                </div>
                <div>
                  <dt className="font-medium text-slate-900">expires at</dt>
                  <dd>{session.expires_at}</dd>
                </div>
              </dl>
            </>
          ) : (
            <p className="text-sm text-slate-600">
              no active session yet. enter the bridge host and create one to start the
              approval loop.
            </p>
          )}
        </div>
        <InfoCard
          title="manual host entry"
          body="the session flow needs a bridge host up front because the browser still connects directly to the home server rather than through cloudflare."
        />
      </section>
    </div>
  );
}

function SettingsPage({ profile }: { profile: StoredServerProfile }) {
  return (
    <div className="grid gap-6 lg:grid-cols-[0.92fr_1.08fr]">
      <section className="panel p-6 sm:p-8">
        <p className="pill">settings</p>
        <h2 className="mt-4 text-3xl font-semibold tracking-tight text-slate-900">
          bridge settings and connection details
        </h2>
        <p className="mt-3 text-sm leading-6 text-slate-600">
          operational details live here so the main app can stay focused on chats.
        </p>

        <div className="mt-6 space-y-4">
          <SettingsRow label="bridge" value={profile.name} />
          <SettingsRow label="api host" value={profile.apiBaseUrl} multiline />
          <SettingsRow label="websocket" value={`${profile.wsBaseUrl}/v1/events`} multiline />
          <SettingsRow
            label="tls fingerprint"
            value={profile.tlsFingerprint || 'not captured in this auth flow'}
            multiline
          />
          <SettingsRow label="scopes" value={profile.scopes.join(', ')} />
          <SettingsRow label="token expiry" value={profile.expiresAt} />
        </div>
      </section>

      <section className="space-y-6">
        <div className="panel p-6 sm:p-8">
          <p className="pill">access</p>
          <h3 className="mt-4 text-xl font-semibold text-slate-900">add another session</h3>
          <p className="mt-3 text-sm text-slate-600">
            pairing and delegated approval are tucked in here instead of cluttering the main app.
          </p>
          <div className="mt-5 flex flex-wrap gap-3">
            <Link className="button-primary" to="/pair">
              pair with qr
            </Link>
            <Link className="button-secondary" to="/session">
              approval code
            </Link>
          </div>
        </div>

        <InfoCard
          title="private by design"
          body="the shell still talks directly to the selected bridge over tailscale. this page only reorganizes the noisy bits so the main views feel more like a modern app."
        />
      </section>
    </div>
  );
}

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

  const activeChat = useMemo(
    () => chats.find((chat) => chat.id === activeChatId) ?? null,
    [activeChatId, chats],
  );

  useEffect(() => {
    chatsRef.current = chats;
  }, [chats]);

  useEffect(() => {
    messagesRef.current = messages;
  }, [messages]);

  useEffect(() => {
    activeChatIdRef.current = activeChatId;
  }, [activeChatId]);

  useEffect(() => {
    if (!token || (status !== 'ready' && status !== 'refreshing')) {
      setEventsStatus('idle');
      setEventsError(null);
      return;
    }

    let cancelled = false;
    let socket: WebSocket | null = null;
    let reconnectTimer = 0;
    let reconnectDelay = 1000;

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
        setEventsStatus('error');
        setEventsError('live event stream dropped. retrying…');
      };

      socket.onclose = () => {
        if (cancelled) {
          return;
        }

        setEventsStatus('connecting');
        reconnectTimer = window.setTimeout(() => {
          reconnectDelay = Math.min(reconnectDelay * 2, 15000);
          connect();
        }, reconnectDelay);
      };
    };

    connect();

    return () => {
      cancelled = true;
      window.clearTimeout(reconnectTimer);
      socket?.close();
    };
  }, [profile.wsBaseUrl, status, token]);

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

  return (
    <section className="panel overflow-hidden p-4 sm:p-5">
      <div className="rounded-[30px] bg-gradient-to-br from-white via-white to-slate-100 px-4 py-5 sm:px-5">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div className="min-w-0">
            <p className="pill">conversation view</p>
            <h2 className="mt-4 truncate text-2xl font-semibold tracking-tight text-slate-900 sm:text-3xl">
              {profile.name}
            </h2>
            <p className="mt-2 text-sm text-slate-500">
              cached threads stay interactive while the bridge refreshes in the background.
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <StatusBadge status={eventsStatus} />
            <StatusBadge status={status} />
            <Link className="button-secondary" to="/settings">
              settings
            </Link>
          </div>
        </div>

        {(status === 'loading' || status === 'refreshing' || status === 'error') ? (
          <div className="mt-5 rounded-[24px] bg-white/80 px-4 py-3 text-sm text-slate-600">
            {status === 'loading' ? 'bringing the bridge online for this screen…' : null}
            {status === 'refreshing'
              ? 'showing local state while the bridge refreshes in the background.'
              : null}
            {status === 'error'
              ? `bridge refresh failed: ${error}. the shell will keep showing the last local state it has.`
              : null}
          </div>
        ) : null}
      </div>

      <div className="mt-4 grid gap-4 xl:grid-cols-[360px_minmax(0,1fr)]">
        <ChatList
          activeChatId={activeChatId}
          chats={chats}
          disabled={status === 'loading' && chats.length === 0}
          status={status}
          onSelectChat={(chatId) => {
            if (chatId === activeChatId) {
              return;
            }
            void loadMessagesForChat(chatId);
          }}
        />
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
          status={threadStatus}
          statusBadge={<StatusBadge status={threadStatus} />}
          threadError={threadError}
        />
      </div>
    </section>
  );
}

function ChatList(props: {
  chats: Chat[];
  activeChatId: number | null;
  disabled: boolean;
  status: 'idle' | 'loading' | 'refreshing' | 'ready' | 'error';
  onSelectChat: (chatId: number) => void;
}) {
  return (
    <div className="rounded-3xl border border-slate-200 bg-slate-50 p-4">
      <div className="flex items-center justify-between gap-3">
        <p className="text-sm font-semibold text-slate-900">conversations</p>
        <span className="text-xs uppercase tracking-[0.16em] text-slate-400">
          {props.chats.length} loaded
        </span>
      </div>

      {props.status === 'refreshing' ? (
        <div className="mt-3 rounded-2xl bg-sky-50 px-3 py-2 text-xs text-sky-700">
          refreshing chats in the background…
        </div>
      ) : null}

      {props.status === 'loading' && props.chats.length === 0 ? (
        <div className="mt-3 rounded-2xl bg-white px-3 py-2 text-xs text-slate-500">
          loading conversations…
        </div>
      ) : null}

      <div className="mt-4 space-y-3">
        {props.chats.length === 0 ? (
          <div className="rounded-2xl bg-white px-4 py-5 text-sm text-slate-500">
            no chats came back from the bridge yet.
          </div>
        ) : (
          props.chats.map((chat) => {
            const active = chat.id === props.activeChatId;
            return (
              <button
                key={chat.id}
                className={
                  active
                    ? 'w-full rounded-2xl border border-signal/20 bg-glow px-4 py-3 text-left'
                    : 'w-full rounded-2xl border border-transparent bg-white px-4 py-3 text-left transition hover:border-slate-200 hover:bg-slate-100'
                }
                disabled={props.disabled}
                onClick={() => props.onSelectChat(chat.id)}
                type="button"
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <p className="truncate text-sm font-semibold text-slate-900">
                      {displayChatName(chat)}
                    </p>
                    <p className="mt-1 truncate text-xs text-slate-500">
                      {chat.preview_text || chat.identifier || chat.service || `chat ${chat.id}`}
                    </p>
                  </div>
                  <span className="shrink-0 text-[11px] uppercase tracking-[0.16em] text-slate-400">
                    {formatChatTimestamp(chat.last_message_at)}
                  </span>
                </div>
              </button>
            );
          })
        )}
      </div>
    </div>
  );
}

function ThreadView(props: {
  activeChat: Chat | null;
  canLoadOlder: boolean;
  loadingOlder: boolean;
  messages: Message[];
  onLoadOlder: () => void;
  shellStatus: 'idle' | 'loading' | 'refreshing' | 'ready' | 'error';
  status: 'idle' | 'loading' | 'refreshing' | 'ready' | 'error';
  statusBadge: ReactNode;
  threadError: string | null;
  onReload: () => void;
}) {
  return (
    <div className="rounded-3xl border border-slate-200 bg-white p-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="text-sm font-semibold text-slate-900">
            {props.activeChat ? displayChatName(props.activeChat) : 'thread'}
          </p>
          <p className="mt-1 text-xs text-slate-500">
            {props.activeChat
              ? props.activeChat.identifier || props.activeChat.service || `chat ${props.activeChat.id}`
              : 'pick a conversation to load recent history'}
          </p>
        </div>
        <div className="flex items-center gap-2">
          {props.statusBadge}
          <button
            className="button-secondary !px-3 !py-2 text-xs"
            disabled={!props.activeChat}
            onClick={props.onReload}
            type="button"
          >
            reload
          </button>
        </div>
      </div>

      <div className="mt-4 min-h-[28rem] space-y-3 rounded-3xl bg-slate-50 p-4">
        {props.status === 'refreshing' ? (
          <div className="rounded-2xl bg-sky-50 px-4 py-3 text-sm text-sky-700">
            showing cached messages while the bridge refreshes this thread…
          </div>
        ) : null}

        {props.activeChat && props.canLoadOlder ? (
          <div className="flex justify-center">
            <button
              className="button-secondary !px-3 !py-2 text-xs"
              disabled={props.loadingOlder}
              onClick={props.onLoadOlder}
              type="button"
            >
              {props.loadingOlder ? 'loading older…' : 'load older messages'}
            </button>
          </div>
        ) : null}

        {props.status === 'idle' ? (
          <div className="rounded-2xl bg-white px-4 py-3 text-sm text-slate-500">
            waiting for a conversation to load.
          </div>
        ) : null}

        {props.status === 'loading' ? (
          props.messages.length > 0 ? (
            <div className="rounded-2xl bg-white px-4 py-3 text-sm text-slate-500">
              pulling recent messages from the bridge…
            </div>
          ) : (
            <LoadingStack />
          )
        ) : null}

        {props.status === 'error' ? (
          <div className="rounded-2xl bg-rose-50 px-4 py-3 text-sm text-rose-700">
            thread load failed: {props.threadError}
          </div>
        ) : null}

        {props.status === 'ready' && props.messages.length === 0 ? (
          <div className="rounded-2xl bg-white px-4 py-3 text-sm text-slate-500">
            this conversation is reachable, but there are no messages in the current result set.
          </div>
        ) : null}

        {props.status === 'ready' && props.threadError ? (
          <div className="rounded-2xl bg-amber-50 px-4 py-3 text-sm text-amber-800">
            {props.threadError}
          </div>
        ) : null}

        {props.messages.map((message) => (
          <MessageBubble key={message.guid || String(message.id)} message={message} />
        ))}
      </div>

      <div className="mt-4 rounded-2xl border border-dashed border-slate-200 bg-white px-4 py-3 text-sm text-slate-500">
        {props.shellStatus === 'error'
          ? 'the bridge refresh failed, so the shell is showing the last local state it had.'
          : 'write actions are still separate. this screen is the real read path first.'}
      </div>
    </div>
  );
}

function LoadingStack() {
  return (
    <div className="space-y-3">
      {[0, 1, 2, 3].map((row) => (
        <div key={row} className={row % 2 === 0 ? 'flex justify-start' : 'flex justify-end'}>
          <div className="w-[72%] animate-pulse rounded-3xl bg-white px-4 py-4 shadow-sm">
            <div className="h-2.5 w-20 rounded-full bg-slate-200" />
            <div className="mt-3 h-3.5 w-full rounded-full bg-slate-200" />
            <div className="mt-2 h-3.5 w-2/3 rounded-full bg-slate-200" />
          </div>
        </div>
      ))}
    </div>
  );
}

function MessageBubble({ message }: { message: Message }) {
  const fromMe = message.is_from_me;
  const text = message.text?.trim();
  const attachments = message.attachments ?? [];
  const reactions = message.reactions ?? [];

  return (
    <div className={fromMe ? 'flex justify-end' : 'flex justify-start'}>
      <article
        className={
          fromMe
            ? 'max-w-[85%] rounded-3xl rounded-br-xl bg-signal px-4 py-3 text-white shadow-sm'
            : 'max-w-[85%] rounded-3xl rounded-bl-xl bg-white px-4 py-3 text-slate-900 shadow-sm'
        }
      >
        <div className="flex items-center justify-between gap-4">
          <p className={fromMe ? 'text-xs font-medium uppercase tracking-[0.16em] text-white/70' : 'text-xs font-medium uppercase tracking-[0.16em] text-slate-400'}>
            {fromMe ? 'you' : message.sender || 'contact'}
          </p>
          <p className={fromMe ? 'text-xs text-white/70' : 'text-xs text-slate-400'}>
            {formatMessageTimestamp(message.created_at)}
          </p>
        </div>

        {text ? <p className="mt-3 whitespace-pre-wrap text-sm leading-6">{text}</p> : null}

        {!text && attachments.length > 0 ? (
          <p className={fromMe ? 'mt-3 text-sm text-white/80' : 'mt-3 text-sm text-slate-500'}>
            attachment-only message
          </p>
        ) : null}

        {attachments.length > 0 ? (
          <div className="mt-3 flex flex-wrap gap-2">
            {attachments.map((attachment, index) => (
              <span
                key={attachment.id || `${message.id}-${index}`}
                className={
                  fromMe
                    ? 'rounded-full bg-white/15 px-3 py-1 text-xs text-white'
                    : 'rounded-full bg-slate-100 px-3 py-1 text-xs text-slate-600'
                }
              >
                {attachment.filename || 'attachment'}
                {attachment.size_bytes ? ` · ${formatBytes(attachment.size_bytes)}` : ''}
              </span>
            ))}
          </div>
        ) : null}

        {reactions.length > 0 ? (
          <p className={fromMe ? 'mt-3 text-xs text-white/70' : 'mt-3 text-xs text-slate-500'}>
            {reactions
              .map((reaction) => reaction.emoji || reaction.type || 'reaction')
              .join(' ')}
          </p>
        ) : null}
      </article>
    </div>
  );
}

function StatusBadge({
  status,
}: {
  status:
    | 'idle'
    | 'loading'
    | 'refreshing'
    | 'ready'
    | 'error'
    | 'connecting'
    | 'live';
}) {
  const styles: Record<typeof status, string> = {
    idle: 'bg-slate-100 text-slate-500',
    loading: 'bg-amber-100 text-amber-700',
    refreshing: 'bg-sky-100 text-sky-700',
    ready: 'bg-glow text-signal',
    connecting: 'bg-sky-100 text-sky-700',
    live: 'bg-glow text-signal',
    error: 'bg-rose-100 text-rose-700',
  };

  return <span className={`rounded-full px-3 py-1 text-xs font-semibold uppercase tracking-[0.16em] ${styles[status]}`}>{status}</span>;
}

function InfoCard(props: { title: string; body: string }) {
  return (
    <div className="panel rounded-[28px] p-6">
      <h3 className="text-lg font-semibold">{props.title}</h3>
      <p className="mt-3 text-sm leading-6 text-slate-600">{props.body}</p>
    </div>
  );
}

function SettingsRow(props: { label: string; value: string; multiline?: boolean }) {
  return (
    <div className="rounded-[24px] bg-white/82 px-4 py-4">
      <p className="text-xs uppercase tracking-[0.18em] text-slate-400">{props.label}</p>
      <p
        className={
          props.multiline
            ? 'mt-2 break-all text-sm leading-6 text-slate-700'
            : 'mt-2 text-sm text-slate-700'
        }
      >
        {props.value}
      </p>
    </div>
  );
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
    return 'recent';
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return 'recent';
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
    return 'unknown time';
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return 'unknown time';
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

function QrScannerPanel({ onPayload }: { onPayload: (value: string) => void }) {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const [status, setStatus] = useState('waiting to start the camera');
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!window.BarcodeDetector) {
      setError('this browser does not expose barcode detection, so paste the qr payload manually');
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
        setStatus('camera is live, looking for a qr code');

        const scan = async () => {
          if (cancelled || !videoRef.current) {
            return;
          }

          try {
            const results = await detector.detect(videoRef.current);
            const payload = results.find((item) => item.rawValue)?.rawValue;
            if (payload) {
              onPayload(payload);
              setStatus('qr payload captured');
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
    <div className="panel p-6 sm:p-8">
      <p className="pill">camera scan</p>
      <div className="mt-4 overflow-hidden rounded-3xl border border-slate-200 bg-slate-950">
        <video className="aspect-square w-full object-cover" muted playsInline ref={videoRef} />
      </div>
      <p className="mt-4 text-sm text-slate-600">{status}</p>
      {error ? (
        <p className="mt-3 rounded-2xl bg-rose-50 px-4 py-3 text-sm text-rose-700">
          {error}
        </p>
      ) : null}
    </div>
  );
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
