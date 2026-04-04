# imsg-bridge

`imsg-bridge` is an authenticated https api for reading and sending iMessage data over tailscale.

this repo is starting with phase 1: a small daemon, self-signed tls, `imsg` subprocess wrappers, and the read-only api surface.

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
