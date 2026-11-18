# Ravan — personal tunnel reliability edition

Based on BackPack by Amin Mohammadi (AminMGMT)
https://github.com/AminMGMT/BackPack

Ravan is an independent AGPL-3.0 fork. Its compatibility baseline is the published
BackPack **v1.8.5** (`ff86354bcafbd054e7888e48343145323c9b2207`), rather than the
unreleased upstream v2 tree. The focus is a WSS Mux tunnel that recovers after
connections go stale, without requiring the Iran service to be restarted by hand.
It is not an official upstream release and does not claim to fix every network outage.

## Install

The attached prebuilt bundles support Linux amd64 and arm64. On both servers, as root:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/sabialballa/Ravan/main/install.sh)
```

Open the English menu later with:

```sh
sudo ravan
```

Use **Setup Iran → Reverse → WebSocket → WSS Mux**, copy the generated setup link, and use
**Setup Kharej → Reverse → WebSocket → WSS Mux → Setup Link** on the other server. No SSH
between the servers or manual TOML editing is required for that setup flow.
Use the same Ravan release at both ends. Choose ports not used by an existing tunnel.

The installer downloads a checksum-verified release. Offline installs require the
matching `SHA256SUMS`; an archive with no checksum, a missing checksum entry or a
wrong digest is refused. Downloads show a single progress bar and have time limits.
Release signatures are verified by the in-app updater with Ravan's own public key.

## Existing installations

Ravan uses `/usr/local/bin/ravan`, `/etc/ravan`, `/root/Ravan` and `ravan-*.service`.
It does not overwrite the upstream binary, configurations or service names.
Do not reuse the live tunnel's listening/forwarded ports for a parallel test.
Build and verify a separate tunnel, then plan the port cutover; nothing in this
repository automatically migrates or stops your existing services.

## Reliability changes

- Backports upstream recovery commit
  [`6e6fd6e`](https://github.com/AminMGMT/BackPack/commit/6e6fd6e6cf0eaa9ce6f277b5dab87d7bc25b89a3):
  cancelled sessions and workers are closed and joined, old generations cannot
  corrupt replacement counters, dead pool entries are retired, and queued
  connections do not leave permanent quota leaks. The original authors retain credit.
- Bounds the Iran side's WebSocket heartbeat acknowledgement wait. A socket that
  still accepts writes but receives no replies is retired after roughly 30–50
  seconds. Healthy idle tunnels keep their heartbeat acknowledgements and are not
  restarted merely because no users are sending data.
- Keeps WSS setup, authentication and TLS behaviour from the stable baseline;
  does not introduce the incompatible v2 PCK rewrite.
- Uses separate release identity, keys, product marks and installation locations.
- Reverse PCK checks round-trip control replies and sends its recovery goodbye
  before closing a timed-out seat, allowing the client to redial with fresh ports.
- Its client learns periodic PCK probes separately from heartbeats, reducing
  server-crash detection delay while retaining the legacy server's timing policy.

## Verification and limits

The v0.1.5 Reverse PCK fix has a separate
[PCK/WSS Mux recovery comparison](docs/reverse-pck-recovery.md), including its
limited-load real-path scope. The original validation report below is historical.

The v0.1.6 client change and transient-probe investigation are recorded in
[PCK crash recovery](docs/pck-crash-recovery.md).

See [reliability notes](docs/ravan.md) and the [validation report](docs/ravan-validation.json) for exact
tests and their limits. Local fault-injection and reconnect tests cannot certify a
particular Iran–abroad route for several hours. There is no timed periodic restart
pretending to solve the failure. A peer that is down or a blocked route remains down.

The first full-suite invocation was not entirely green. Corrective reruns,
KCP cold-start allowances in software emulation, and absolute timing-gate results
are recorded explicitly in that report; this initial personal release is not a
claim of production-route or hardware-performance certification.

## Build

```sh
CGO_ENABLED=0 go build -trimpath -o ravan .
go test ./...
go test -race ./...
```

See `go.mod` for the toolchain requirement. The inherited documentation and
[upstream README](README_UPSTREAM.md) are retained as dated reference; command,
path, repository and release names in those historical pages belong to upstream.

## Licence

AGPL-3.0. Preserve [LICENSE](LICENSE), [NOTICE](NOTICE), the upstream attribution
above and applicable notices when distributing modifications. Upstream marks are
not Ravan's branding; see [TRADEMARK.md](TRADEMARK.md). Ravan uses its own mark.

New connection-test and tunnel setup links use `ravan://`. Ravan also accepts older
`backpack://` links. Update both servers before pasting a newly generated Ravan link;
older binaries do not recognize the new prefix. Generated install commands use
`https://raw.githubusercontent.com/sabialballa/Ravan/main/install.sh`.
