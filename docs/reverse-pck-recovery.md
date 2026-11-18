# Reverse PCK recovery

Last verified against Ravan v0.1.6.

Reviewed for v0.1.6. The hour-long measurements below remain the v0.1.5 record.
Client-side crash recovery and transient-probe diagnostics are documented in
[PCK crash recovery](pck-crash-recovery.md).

## Fix

Reverse PCK is KCP over raw TCP-shaped packets. Successful writes can mean
only that KCP queued bytes; server-to-client heartbeats do not prove that
the client-to-server direction still works. The server now sends the existing
RTT request every five seconds and bounds replies by twenty seconds. v0.1.4
clients already understand and echo that request. Ordinary KCP, XDI and Spoof
do not acquire a new acknowledgement requirement.

On a probe timeout, the control loop now sends and flushes its goodbye before
vacating the client seat closes the channel. When the outgoing direction still
works, this tells the client to redial from new raw source ports immediately,
instead of adding its own heartbeat deadline after the server's timeout.
The listener and forwarded listening ports remain open. Connections already
in flight can be interrupted; this is recovery, not seamless failover.

This is a server/Iran-side change for **Reverse PCK**. The v0.1.3 Direct PCK
change is a separate dialler-side policy. Updating both endpoints is recommended.
No packet format, token, cipher or configuration key changes.

## Validation scope

The validation record separates a native Linux namespace comparison from a
real Iran/outside path. They are not interchangeable bandwidth measurements.
Both use simultaneous PCK and WSS Mux workloads with exact echo-byte checks.
Faults cover one-way flow blackholes, bidirectional interruption and process
crashes. A control-flow blackhole may leave pooled data connections working;
control redial time and application availability must therefore be reported
separately.

The real-path experiment shares the outside server with existing services.
The complete experiment process group is capped at 10% of one CPU core and
512 MiB on each host. Offered application load is about 1 Mbit/s aggregate
across both modes and directions, below the user's 2 Mbit/s budget. This is a
constrained recovery comparison, not a maximum-throughput benchmark. The test
filters match only the peer and temporary test ports; existing services and
root queue disciplines are preserved.

Recovery timings are derived from timestamped probe samples and control logs.
An in-flight reply around rule installation is not counted as recovery from a
subsequent observed outage. No claim of whole-repository race certification,
multi-day stability or universal ISP behavior is made.

## Results — 18 November 2026

The native Linux run completed **3600.196 seconds and ten injected faults**.
Both modes recovered after every fault and ended with fresh successful traffic.
Exact workload echo checks recorded zero corrupt payloads. This run included
two repetitions each of outgoing-only control-flow loss, return-only control-flow
loss, a full path interruption, server crash and client crash. Its repeated
one-way PCK control claims advanced rather than retaining the blackholed seat.

The real Iran/outside comparison used a separate **3600-second** window with
both modes running simultaneously. Application recovery below is measured from
the actual filter installation/process stop to a successful probe after an
observed failure. It includes the injected outage itself; it is not extra delay
after connectivity returns. A working pooled stream can outlive its control flow.

| Injected fault | PCK application recovery | WSS Mux application recovery | PCK control redial | WSS Mux control redial |
|---|---:|---:|---:|---:|
| outgoing | 23.2 s | 28.1 s | 20.6 s | 25.6 s |
| return | 31.2 s | 28.1 s | 29.6 s | 26.5 s |
| both | 22.7 s | 22.6 s | 21.8 s | not observed |
| server_crash | 31.0 s | 7.5 s | 28.9 s | 5.8 s |
| client_crash | 6.8 s | 6.9 s | 5.9 s | 4.9 s |

- **Reverse PCK:** 211,394,560 bidirectional payload bytes, 6,875 successful probes, 83 failed probes including injected faults, zero corrupt workload payloads. 2 failed probes occurred outside the injected-fault windows plus 60 seconds of recovery allowance.
- **WSS Mux:** 200,019,968 bidirectional payload bytes, 6,920 successful probes, 38 failed probes including injected faults, zero corrupt workload payloads. 2 failed probes occurred outside the injected-fault windows plus 60 seconds of recovery allowance.

Both modes recovered without a manual service restart. The isolated server-crash
case favored WSS Mux: about 5.5–6.0 seconds versus PCK's 28.7–32.7 seconds.
These results support the one-way PCK repair, not a claim that PCK is always
faster or that either mode has zero interruptions. The constrained outside CPU,
existing service load and small offered workload limit throughput conclusions.
The real bidirectional cut was installed independently on two hosts: its union
was about 22 seconds although each local filter was held for about 15 seconds.
Actual fault intervals are retained in the machine-readable record.

Short namespace pilots reproduced the old behavior: the v0.1.4 PCK control seat
never redialed under outgoing-only control loss, although application probes
eventually recovered through pooled connections (about 71 seconds in that pilot).
With the fixed server and a v0.1.4 client, a separate pilot recovered application
probes in about 18 seconds. These short warm-up pilots are compatibility and bug
reproduction evidence, not controlled throughput or universal speedup claims.

Linux validation also passed the complete `internal/server/transport` race run
(20.936 seconds), focused recovery tests, changed-package vet/staticcheck,
documentation/attribution checks, and an opt-in namespace wrapper run using the
previous client (138.77 seconds including an outgoing fault). No full-repository
race certification is implied.

The hour tests used pre-release candidate SHA-256
`364f9ea7e72ff71759ae7ed038cd3f915da53f5ed503bdb45a95922a83553953`.
Its version label preceded the final v0.1.5 metadata bump. Tagged release binaries
are rebuilt from the same transport changes and receive a separate installation
and smoke check; the final archive is not claimed to have run this hour itself.
The first native fault's raw recovery summary admitted an in-flight reply around
filter installation; reported native recovery was recomputed from raw samples
after an observed failed probe. The published reproduction helper excludes that
in-flight case. Transport code was unchanged during both hour runs.

See [the compact measurements](reverse-pck-validation.json). The release includes
`ravan-reverse-recovery-evidence.json.gz` with sanitized timestamped samples and
events; it omits addresses, configuration credentials and private TLS material.
Its digest is covered by the signed release checksum manifest.

## Reproduction in an isolated Linux lab

```sh
RAVAN_REVERSE_RECOVERY_LIVE=1 go test ./internal/e2e \
  -run TestReversePCKRecoveryComparedWithWSSMux -v -timeout 70m
```

The default is a real 3600-second workload, not an accelerated timer. The
harness refuses execution without separate network and mount namespaces.
Set `BACKPACK_PREV_BINARY` to test a previous client against the new server.
Short `RAVAN_REVERSE_RECOVERY_SECONDS` runs are functional checks, not one-hour
acceptance runs. Linux raw-socket, namespace, iproute2 and OpenSSL support are
required. The optional harness does not run in ordinary unit tests.

## خلاصهٔ فارسی

<div dir="rtl">

در Reverse PCK، رسیدن پیام از یک جهت کافی نبود تا سلامت رفت‌وبرگشت مشخص شود.
سرور ایران اکنون مرتب پاسخ رفت‌وبرگشت می‌خواهد و هنگام خرابی، پیش از بستن کانال،
پیام اتصال مجدد را می‌فرستد. تنظیمات و کلیدهای فعلی حفظ می‌شوند. بازیابی ممکن است
اتصال‌های در حال انتقال را قطع کند؛ اتصال بدون وقفه تضمین نمی‌شود. آزمون مسیر واقعی
با سقف مصرف تعیین‌شده اجرا می‌شود و معیار سرعت حداکثری اینترنت نیست.

</div>
