# imsg-bridge

`imsg-bridge` is a small https api for iMessage over tailscale.

it runs on a Mac, talks to [`imsg`](https://github.com/steipete/imsg), and gives paired clients a clean way to read chats, stream events, send messages, and manage webhooks without exposing the machine to the public internet.

## what it does

today, `imsg-bridge` supports:

- tls on first boot with a pinned self-signed fingerprint
- token-based auth with direct pairing and delegated web sessions
- read endpoints for server info, chats, and message history
- a websocket event stream backed by `imsg watch`
- local admin over a unix socket with `imsg-bridge-cli`
- webhook registration and delivery
- api-based message and attachment sending

still landing:

- a more polished install story for running it as a background service

## what you need

- macOS 14 or newer
- Messages signed in and working
- full disk access for the terminal or daemon
- [`imsg`](https://github.com/steipete/imsg): `brew install imsg`
- [tailscale](https://tailscale.com/) installed and connected

for local bridge work, the daemon also accepts `IMSGBRIDGE_IMSG_BIN=/path/to/imsg` or `-imsg-bin /path/to/imsg` if you need to point at a patched checkout before upstream catches up.

## quick start

```sh
make fmt
make test
make build
make build-cli
make run
```

the server listens on `:8443` by default.

on first boot it will:

- create a self-signed cert in the data dir
- log the tls fingerprint used for pairing
- create a short-lived bootstrap pairing code if no clients exist yet

after that, local admin happens over the unix socket at `~/.local/share/imsg-bridge/imsg-bridge.sock`.

## how to use it

there are really two client modes today:

- direct clients: cli tools, native apps, and anything that can live with the bridge's self-signed tls plus the pairing fingerprint
- browser clients: the web app shell, which needs a browser-trusted https host and should go through tailscale serve

### direct clients

direct clients can talk to the bridge itself on `:8443`.

the normal path is:

```sh
imsg-bridge-cli pair
```

that command mints a pairing code, prints the server fingerprint, and renders a terminal qr that direct clients can scan.

common local commands:

```sh
imsg-bridge-cli status
imsg-bridge-cli pair
imsg-bridge-cli clients
imsg-bridge-cli revoke c_01example
```

for local bridge work, you can also point the daemon at a patched `imsg` checkout:

```sh
IMSGBRIDGE_IMSG_BIN=/path/to/imsg make run
```

or:

```sh
./bin/imsg-bridge -imsg-bin /path/to/imsg
```

### browser clients

the browser should not connect to the bridge's direct `:8443` listener.

that listener uses a self-signed cert, which is fine for pinned native flows but not for a normal browser.

for browsers, use:

- cloudflare pages for the static shell
- tailscale serve for the bridge traffic
- the `*.ts.net` hostname as the saved browser host

#### 1. run the bridge locally

```sh
make run
```

or your built binary:

```sh
./bin/imsg-bridge
```

#### 2. put tailscale serve in front of it

if your bridge is listening on `127.0.0.1:8443` or `:8443` with its current self-signed cert, this is the simplest setup:

```sh
tailscale serve --https=443 https+insecure://127.0.0.1:8443
tailscale serve status
```

that gives you a browser-trusted tailscale https endpoint, usually something like:

```text
https://bridge-name.your-tailnet.ts.net
```

#### 3. deploy the web shell

the web app is just a static frontend:

```sh
cd web
npm install
npm run build
```

then deploy `web/dist` to cloudflare pages.

cloudflare is only hosting the shell. it is not proxying message traffic.

#### 4. pair from the browser

open the cloudflare-hosted web app and use either:

- `pair with qr`
- `approval code`

for the browser host, use the plain `*.ts.net` host:

```text
bridge-name.your-tailnet.ts.net
```

not:

```text
bridge-name.your-tailnet.ts.net:8443
100.x.y.z:8443
https://100.x.y.z:8443
```

the web app stores:

- `https://bridge-name.your-tailnet.ts.net` for api requests
- `wss://bridge-name.your-tailnet.ts.net` for live events

### delegated browser login

web clients can also use the delegated session flow instead of qr pairing:

```sh
curl -sk https://127.0.0.1:8443/v1/sessions \
  -H 'Content-Type: application/json' \
  -d '{"client_name":"Chrome","client_type":"web"}'
```

the browser polls the returned `session_id`, and an already-paired client approves it.

## pairing a client

the short version:

- use `imsg-bridge-cli pair` for direct/native clients
- use the cloudflare-hosted web shell plus your `*.ts.net` host for browser clients
- do not use `:8443` as the browser host

## api shape

main endpoints:

- `GET /healthz`
- `GET /v1/server`
- `GET /v1/chats`
- `GET /v1/chats/{id}/messages`
- `POST /v1/pair`
- `POST /v1/sessions`
- `GET /v1/sessions/{id}`
- `POST /v1/sessions/{id}/approve`
- `GET /v1/events`
- `POST /v1/messages`
- `POST /v1/attachments`
- `GET /v1/attachments/{id}`
- `GET /v1/webhooks`
- `POST /v1/webhooks`
- `DELETE /v1/webhooks/{id}`

example send request:

```sh
curl -sk https://127.0.0.1:8443/v1/messages \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"to":"+15551234567","text":"hi from imsg-bridge","service":"auto"}'
```

example webhook list request:

```sh
curl -sk https://127.0.0.1:8443/v1/webhooks \
  -H "Authorization: Bearer $TOKEN"
```

example attachment send request:

```sh
curl -sk https://127.0.0.1:8443/v1/attachments \
  -H "Authorization: Bearer $TOKEN" \
  -F to=+15551234567 \
  -F text='photo attached' \
  -F file=@./photo.jpg
```

webhook targets must use `https://` and cannot resolve to loopback, private, link-local, or metadata addresses.

## browser troubleshooting

if desktop works but mobile chrome fails, check these first:

- the saved browser host must be your `*.ts.net` hostname with no `:8443`
- `tailscale serve status` should show port `443` serving the bridge
- if you just deployed the web shell, do one hard refresh so the browser picks up the latest service worker and bundle

if the bridge log shows:

```text
tls: unknown certificate
```

or:

```text
tls handshake error ... EOF
```

the browser is almost always talking to the bridge's direct self-signed tls listener instead of the tailscale serve host.

## project notes

- the daemon keeps its state in `~/.local/share/imsg-bridge/`
- the api is meant for tailscale clients, not direct public exposure
- websocket auth uses `?token=` because browser websocket clients cannot set custom auth headers
- for now, the browser story assumes tailscale serve and a `*.ts.net` hostname. direct self-signed tls is for native/direct clients.

if you want the deeper architecture and rollout plan, that lives in the repo notes rather than this README.
