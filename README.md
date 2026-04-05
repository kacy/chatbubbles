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

## pairing a client

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

web clients use a delegated session flow instead of qr pairing:

```sh
curl -sk https://127.0.0.1:8443/v1/sessions \
  -H 'Content-Type: application/json' \
  -d '{"client_name":"Chrome","client_type":"web"}'
```

the browser polls the returned `session_id`, and an already-paired client approves it.

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

## project notes

- the daemon keeps its state in `~/.local/share/imsg-bridge/`
- the api is meant for tailscale clients, not direct public exposure
- websocket auth uses `?token=` because browser websocket clients cannot set custom auth headers

if you want the deeper architecture and rollout plan, that lives in the repo notes rather than this README.
