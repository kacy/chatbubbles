# imsg-bridge

`imsg-bridge` is an authenticated https api for reading and sending iMessage data over tailscale.

this repo now has the phase 1 and phase 2 foundations plus the first auth bootstrap path: a small daemon, self-signed tls, `imsg` subprocess wrappers, a websocket event stream backed by `imsg watch`, and token-based api access via pairing.

## prerequisites

- macOS 14+
- `imsg` installed with `brew install imsg`
- messages signed in
- full disk access granted to the terminal or daemon
- tailscale installed and connected

## quick start

```sh
make fmt
make test
make run
```

the server listens on `:8443` by default.

on first boot it creates a self-signed certificate in the data dir and logs the tls fingerprint for pairing clients later.

the daemon also keeps a tiny `config.json` in the data dir so the watcher can resume from the last seen row id after a restart.

on a fresh boot with no paired clients, the daemon logs a short-lived bootstrap pairing code. after that, use `imsg-bridge-cli pair` over the local control socket to mint more pairing codes without restarting the daemon.

`imsg-bridge-cli pair` now prints the code, the pinned tls fingerprint, and a terminal qr payload that direct clients can scan.

web clients can also use the delegated session flow:

```sh
curl -sk https://127.0.0.1:8443/v1/sessions \
  -H 'Content-Type: application/json' \
  -d '{"client_name":"Chrome","client_type":"web"}'
```

poll the returned `session_id`, then approve it from an already-paired client with `send` scope.

admin-scoped clients can also manage webhooks:

```sh
curl -sk https://127.0.0.1:8443/v1/webhooks \
  -H "Authorization: Bearer $TOKEN"
```

webhook targets must be `https://` and cannot resolve to loopback, private, link-local, or metadata addresses.

message sending is now available on the api side too:

```sh
curl -sk https://127.0.0.1:8443/v1/messages \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"to":"+15551234567","text":"hi from imsg-bridge","service":"auto"}'
```

## local admin

the daemon also exposes a unix socket at `~/.local/share/imsg-bridge/imsg-bridge.sock` for local admin commands.

```sh
imsg-bridge-cli status
imsg-bridge-cli pair
imsg-bridge-cli clients
imsg-bridge-cli revoke c_01example
```
