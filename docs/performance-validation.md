# Performance and protocol validation

Last verified against Ravan v0.1.7.

Reviewed for v0.1.7. Historical measurements below are unchanged; Reverse PCK recovery is validated separately.

The measurements below remain the v0.1.2 performance record. The PCK recovery
limitation is addressed separately in [v0.1.3 PCK recovery validation](pck-recovery.md).
No new throughput comparison is claimed in v0.1.3.

## Changes and result

Stealth reuses its framed receive buffer and decrypts in place with AEAD authentication.
Unread plaintext survives small caller reads; tampered records do not return plaintext.
Disabled Tracef calls in the shared TCP/WebSocket relays no longer allocate boxed
byte counts per chunk. WebSocket retry backoff now wakes immediately on cancellation.
No wire format, cipher, authentication requirement or performance-preset value changed.

The clear sustained gain in this experiment is Stealth: about 22% higher median
aggregate throughput and about 19% less process CPU for the same transfer. Its
transient allocation total fell from about 1047 MiB to 20 MiB for 512 MiB carried through
the tunnel. These are allocation totals, not resident-memory figures. The in-memory
32 KiB record benchmark fell from 73760 to 32 allocated bytes per operation; a separate
no-op-writer benchmark eliminated two disabled-log allocations per relay chunk.
That no-op benchmark is not a network-speed measurement.

## Native Linux comparison

A baseline build from v0.1.1 and the candidate used the identical harness and
Turbo's 2 MiB mux stream window. Three trials per build ran in ABBAAB order on an
8-vCPU Ubuntu 24.04 VPS, with two Go workers and no public network interfaces inside
the benchmark namespaces. Each trial echoed 32 MiB eight times, 512 MiB total across
the tunnel, with reusable receiver storage. Values below are median aggregate
bidirectional Mbit/s. Both tunnel ends and the echo backend ran in one process.

| Transport | v0.1.1 | Candidate | Median change |
|---|---:|---:|---:|
| tcp | 4453 | 4604 | +3.4% |
| tcpmux | 3701 | 3588 | -3.1% |
| ws | 4046 | 3868 | -4.4% |
| wss | 2367 | 2269 | -4.2% |
| wsmux | 3462 | 3332 | -3.8% |
| wssmux | 2420 | 2285 | -5.6% |
| stealth | 1738 | 2126 | +22.4% |

Non-Stealth ranges overlapped, and several candidate medians were lower. These
observations are retained rather than advertised as gains. There is no claim that
every transport became faster. Shorter nine-mode native and emulated measurements
are also retained in [the full record](performance-validation.json); their especially
short native TCP samples were too variable to support a universal speed claim.
These figures do not measure an Iran-abroad route or an Internet uplink.

## Protocol coverage

- IPv4 and IPv6: TCP,TCPmux,Stealth,WS,WSS,WSmux,WSSmux,KCP,QUIC and UDP.
- UDP forwarding inside all nine stream transports, including WSS,WSSmux and Stealth;
  with basic datagram echo in each mode. Shared framing/source-separation stress
  checks used TCPmux/TCP; incorrect tokens were also refused.
- Reverse XDI/PCK in isolated network and mount namespaces, 256 KiB each way verified.
- Direct GRE/TUN over UDP,QUIC,PCK,SNI,XDI and Spoof: twelve scenarios with 5 MiB
  byte-exact transfers, listener restarts, a blocked PCK flow and lossy/delayed paths.
  Spoof remains a direct carrier; its removed reverse mode was not reinstated.
- Twenty mixed v0.1.1/candidate old/new endpoint checks passed.
- Native Linux race checks passed for the ten-mode data matrix, forwarded UDP and
  WSS/WSSmux TLS, plus focused record, padding, authentication and cancellation tests.

The emulated all-mode race invocation failed KCP timing checks twice. The same race
cases passed on native Linux. Neither these failures nor the excluded absolute
relay-pool timing gate have been removed or relaxed. No whole-repository race or
entirely green unrestricted full-suite result is claimed.

## Remaining limits and operation

PCK took 156.2 seconds to recover when only its outgoing flow was blocked. Listener
restart recovery was about 24 seconds for UDP and 25 seconds for PCK. The asymmetric
PCK delay is a remaining limitation, not a speed fix in this release. Raw-carrier
IPv6 and ARM hardware execution were not tested. No new multi-hour public-route
acceptance run was performed. All temporary VPS test processes, files, keys and
units were removed; the existing service remained running.

Existing tunnel configurations keep their settings. An old mux tunnel retaining a
small per-stream window may remain slow on a high-RTT path after a binary update.
Reapply the desired preset on both ends deliberately; Turbo currently uses 2 MiB per
stream and Aggressive consumes more resources. Increasing buffers blindly is not
an automatic route-speed improvement. Keep TCP autotuning enabled by default.

Reproduce the opt-in comparison with RAVAN_PERFORMANCE=1, optionally
RAVAN_PERFORMANCE_MIB=32 and RAVAN_PERFORMANCE_PASSES=8, then run
`go test ./internal/e2e -run TestPerformanceMatrix -v -count=1`.
Raw tests require RAVAN_RAW_LIVE=1 and suitable Linux namespace tools; the test
script refuses to run without separate network and mount namespaces.

## خلاصهٔ فارسی

<div dir="rtl">

همهٔ حالت‌های موجود در محیط Linux بررسی شدند؛ داده، UDP داخل TCP، IPv6، اتصال هم‌زمان
و سازگاری با نسخهٔ قبلی آزموده شد. بهبود روشن سرعت مربوط به Stealth بود و هزینهٔ حافظهٔ
مسیر دریافت آن به‌شدت کم شد. افزایش سرعت یکسان برای همهٔ پروتکل‌ها ثبت نشد.
اعداد جدول مربوط به شبکهٔ آزمایشی داخلی‌اند، نه سرعت اینترنت یا مسیر ایران به خارج.
بازیابی مسیر یک‌طرفهٔ مسدودشده در PCK حدود۱۵۶ثانیه طول کشید و هنوز محدودیت دارد.
تنظیمات تونل‌های قدیمی خودکار عوض نمی‌شوند؛ پروفایل مناسب را با توجه به منابع هر دو سرور انتخاب کنید.

</div>
