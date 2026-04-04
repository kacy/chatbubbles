import { useEffect, useMemo, useRef, useState } from 'react';
import { Link, Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom';

import {
  createSession,
  fetchServerInfo,
  pairClient,
  pollSession,
} from './lib/api';
import { decryptString, encryptString } from './lib/crypto';
import {
  deleteProfile,
  getActiveProfileId,
  listProfiles,
  saveProfile,
  setActiveProfile,
} from './lib/db';
import { buildProfileDraft, deriveApiBaseUrl } from './lib/connection';
import { parsePairPayload } from './lib/qr';
import type {
  CreateSessionResponse,
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
    <div className="mx-auto min-h-screen max-w-6xl px-4 py-6 sm:px-6 lg:px-8">
      <TopBar activeProfile={activeProfile} />
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
    </div>
  );
}

function TopBar({ activeProfile }: { activeProfile: StoredServerProfile | null }) {
  const location = useLocation();

  return (
    <header className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <p className="pill">direct over tailscale</p>
        <h1 className="mt-3 text-3xl font-extrabold tracking-tight">imsg-bridge web</h1>
        <p className="mt-2 max-w-2xl text-sm text-slate-600">
          cloudflare hosts the shell. this app talks straight to the selected
          home bridge over tailscale.
        </p>
      </div>
      <nav className="flex flex-wrap gap-2">
        <NavLink to="/" currentPath={location.pathname}>
          profiles
        </NavLink>
        <NavLink to="/pair" currentPath={location.pathname}>
          pair
        </NavLink>
        <NavLink to="/session" currentPath={location.pathname}>
          session
        </NavLink>
        <NavLink to="/app" currentPath={location.pathname}>
          app
        </NavLink>
        {activeProfile ? (
          <span className="inline-flex items-center rounded-2xl bg-white/70 px-3 py-2 text-sm font-medium text-slate-700">
            active: {activeProfile.name}
          </span>
        ) : null}
      </nav>
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

function HomePage(props: {
  profiles: StoredServerProfile[];
  activeProfileId: string | null;
  onActivate: (id: string) => Promise<void>;
  onDelete: (id: string) => Promise<void>;
}) {
  return (
    <div className="grid gap-6 lg:grid-cols-[1.2fr_0.8fr]">
      <section className="panel p-6 sm:p-8">
        <div className="flex items-start justify-between gap-4">
          <div>
            <p className="pill">saved bridges</p>
            <h2 className="mt-4 text-2xl font-semibold">server profiles live on this device</h2>
            <p className="mt-3 text-sm text-slate-600">
              profiles, encrypted tokens, and shell state stay local in the browser.
            </p>
          </div>
          <Link className="button-primary" to="/pair">
            add bridge
          </Link>
        </div>

        <div className="mt-6 space-y-4">
          {props.profiles.length === 0 ? (
            <div className="rounded-3xl border border-dashed border-slate-300 bg-slate-50 p-6 text-sm text-slate-600">
              no bridge profiles yet. pair from a qr code or start a delegated session.
            </div>
          ) : (
            props.profiles.map((profile) => (
              <article
                key={profile.id}
                className="rounded-3xl border border-slate-200 bg-white p-5"
              >
                <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                  <div>
                    <h3 className="text-lg font-semibold">{profile.name}</h3>
                    <p className="mt-2 break-all text-sm text-slate-600">{profile.apiBaseUrl}</p>
                    <p className="mt-1 break-all text-xs text-slate-500">
                      tls fingerprint: {profile.tlsFingerprint || 'not captured in this flow'}
                    </p>
                    <p className="mt-3 text-xs uppercase tracking-[0.16em] text-slate-400">
                      scopes: {profile.scopes.join(', ')}
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
          title="how the shell works"
          body="cloudflare pages only serves the app bundle. every api request and websocket connection goes directly to the saved home server over tailscale."
        />
        <InfoCard
          title="what is persisted"
          body="server profiles, encrypted tokens, cache placeholders, and the last active bridge are stored locally so refreshes do not wipe the shell."
        />
        <InfoCard
          title="browser reality"
          body="this shell assumes the browser can directly reach the bridge over tailscale. no relay, no edge proxy, and no public message backend."
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
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [scannerOpen, setScannerOpen] = useState(false);

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError(null);

    try {
      const payload = parsePairPayload(payloadText);
      const apiBaseUrl = deriveApiBaseUrl(payload.h);
      const pairResult = await pairClient(apiBaseUrl, {
        code: payload.c,
        clientName,
        clientType: 'web',
      });

      await props.onSaveProfile({
        profileName: pairResult.server_name,
        host: payload.h,
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
        <h2 className="mt-4 text-2xl font-semibold">scan or paste the bridge qr payload</h2>
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
          body="the shell saves the selected host, websocket base, tls fingerprint, encrypted token, expiry metadata, and local shell state."
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
        <h2 className="mt-4 text-2xl font-semibold">link this browser through another paired device</h2>
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
              placeholder="100.64.0.3:8443"
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

function AppShell({ profile }: { profile: StoredServerProfile }) {
  const [serverInfo, setServerInfo] = useState<ServerInfo | null>(null);
  const [status, setStatus] = useState<'idle' | 'loading' | 'ready' | 'error'>('idle');
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    async function loadServerInfo() {
      setStatus('loading');
      setError(null);

      try {
        const token = await decryptString(profile.token);
        const result = await fetchServerInfo(profile.apiBaseUrl, token);
        if (!cancelled) {
          setServerInfo(result);
          setStatus('ready');
        }
      } catch (loadError) {
        if (!cancelled) {
          setStatus('error');
          setError(
            loadError instanceof Error
              ? loadError.message
              : 'could not reach the selected bridge',
          );
        }
      }
    }

    void loadServerInfo();

    return () => {
      cancelled = true;
    };
  }, [profile]);

  return (
    <div className="grid gap-6 lg:grid-cols-[0.92fr_1.08fr]">
      <aside className="space-y-6">
        <div className="panel p-6 sm:p-8">
          <p className="pill">active bridge</p>
          <h2 className="mt-4 text-2xl font-semibold">{profile.name}</h2>
          <p className="mt-3 break-all text-sm text-slate-600">{profile.apiBaseUrl}</p>
          <dl className="mt-6 space-y-3 text-sm text-slate-600">
            <div>
              <dt className="font-medium text-slate-900">websocket</dt>
              <dd className="break-all">{profile.wsBaseUrl}/v1/events</dd>
            </div>
            <div>
              <dt className="font-medium text-slate-900">scopes</dt>
              <dd>{profile.scopes.join(', ')}</dd>
            </div>
            <div>
              <dt className="font-medium text-slate-900">expires at</dt>
              <dd>{profile.expiresAt}</dd>
            </div>
          </dl>
        </div>

        <InfoCard
          title="cache-first shell"
          body="this route is intentionally a shell for now. the final chat experience will hydrate from cache first, then sync against the bridge."
        />
      </aside>

      <section className="panel p-6 sm:p-8">
        <div className="flex items-start justify-between gap-4">
          <div>
            <p className="pill">app shell</p>
            <h2 className="mt-4 text-2xl font-semibold">messaging surface comes next</h2>
            <p className="mt-3 text-sm text-slate-600">
              the direct connection and persistence foundation are in place. this screen
              is the placeholder for chats, thread view, composer, and live updates.
            </p>
          </div>
          <StatusBadge status={status} />
        </div>

        <div className="mt-8 grid gap-4 lg:grid-cols-[0.9fr_1.1fr]">
          <div className="rounded-3xl border border-slate-200 bg-slate-50 p-4">
            <p className="text-sm font-semibold text-slate-900">chat list placeholder</p>
            <div className="mt-4 space-y-3">
              {['family group', 'project thread', 'notifications'].map((label) => (
                <div key={label} className="rounded-2xl bg-white px-4 py-3 text-sm text-slate-600">
                  {label}
                </div>
              ))}
            </div>
          </div>
          <div className="rounded-3xl border border-slate-200 bg-white p-4">
            <p className="text-sm font-semibold text-slate-900">thread placeholder</p>
            <div className="mt-4 space-y-3 text-sm text-slate-600">
              <div className="rounded-2xl bg-slate-50 px-4 py-3">
                cached messages and sync state will render here.
              </div>
              <div className="rounded-2xl bg-glow px-4 py-3 text-signal">
                composer, attachments, and websocket updates will layer onto this shell.
              </div>
            </div>
          </div>
        </div>

        <div className="mt-8 rounded-3xl border border-slate-200 bg-slate-50 p-4 text-sm text-slate-600">
          {status === 'ready' && serverInfo ? (
            <div className="space-y-2">
              <p className="font-medium text-slate-900">bridge connection looks healthy</p>
              <p>
                server: {serverInfo.name} · app version: {serverInfo.version} · imsg:{' '}
                {serverInfo.imsg_version}
              </p>
            </div>
          ) : null}

          {status === 'loading' ? <p>checking the saved bridge token and server reachability…</p> : null}
          {status === 'error' ? (
            <p className="text-rose-700">
              direct bridge check failed: {error}. this usually means the browser cannot
              currently reach the home server over tailscale or the saved token is no longer valid.
            </p>
          ) : null}
          {status === 'idle' ? <p>waiting to start the direct bridge check…</p> : null}
        </div>
      </section>
    </div>
  );
}

function StatusBadge({ status }: { status: 'idle' | 'loading' | 'ready' | 'error' }) {
  const styles: Record<typeof status, string> = {
    idle: 'bg-slate-100 text-slate-500',
    loading: 'bg-amber-100 text-amber-700',
    ready: 'bg-glow text-signal',
    error: 'bg-rose-100 text-rose-700',
  };

  return <span className={`rounded-full px-3 py-1 text-xs font-semibold uppercase tracking-[0.16em] ${styles[status]}`}>{status}</span>;
}

function InfoCard(props: { title: string; body: string }) {
  return (
    <div className="panel p-6">
      <h3 className="text-lg font-semibold">{props.title}</h3>
      <p className="mt-3 text-sm leading-6 text-slate-600">{props.body}</p>
    </div>
  );
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
