# Ravan reliability notes

Last verified against Ravan v0.1.2.

Certificate changes and their separate validation record are documented in [ACME validation](acme-validation.md). The original reliability results below remain the v0.1.0 baseline.

Performance changes and protocol coverage are recorded separately in [performance validation](performance-validation.md).

## Baseline and change boundaries

The baseline is the published upstream v1.8.5. Recovery fixes from upstream commit
6e6fd6e6cf0eaa9ce6f277b5dab87d7bc25b89a3 are backported with attribution. The initial v0.1.0
change was a read deadline on server-side WebSocket control channels, whose
clients already acknowledge heartbeats. TCP/KCP/QUIC peers that do not acknowledge
heartbeats do not acquire that requirement. The timeout allows several missed beats;
no-traffic time alone is not a failure.

The reported production symptom was WSS Mux stopping after hours, recovered by
restarting Iran. No logs or exact installed upstream version were supplied for that
outage, so these fixes address reproducible failure classes, not a proven exclusive
cause on that machine. A multi-hour Iran–abroad acceptance run remains necessary.

## Failure handling

The control channel can be writable while the return direction has failed. Iran now
expires that client's claim, closes stale resources and can accept the next authenticated
claim without tearing down its forwarded-port listeners. Stream/session counters from
old generations cannot count against the replacement. Stalled setup writes and
cancelled mux sessions have bounded teardown paths.

The monitor is separate from the web panel. Keep its service enabled when using the
watchdog. Its cooldown and failure thresholds are retained; idle tunnels must not be
restarted merely because traffic counters are unchanged. Time-based blanket restarts
are not enabled as a workaround.

## Install and test safely

Install the same Ravan release on both ends. Ravan's files and unit names are separate
from upstream. Set up the test tunnel on unused ports, verify its data plane and
recovery, and only then arrange a deliberate cutover. Do not run two listeners on
the same public or forwarded port. Configurations and setup links contain credentials;
keep them private and redact them from support logs.

For installation without GitHub access, download `install.sh`, the architecture's
`ravan_linux_*.tar.gz` and the matching `SHA256SUMS` to a directory only root/the
operator can write, then run the script there. Missing or mismatching checksums are
refused. Updating also checks the release signature with Ravan's own public key.

## Security maintenance

Restoring a credential-bearing TOML from either a Ravan or a legacy BackPack
backup enforces owner-only permissions (0600), even when its archive mode was
0644. Legacy directory names remain sensitive after the fork's path change.

Release binaries use Go 1.27.4. The dependency scan identified reachable issues in
Brotli and zstd decompression; the fork updates brotli to v1.2.7 and compress to
v1.20.2, together with the affected x/crypto and x/net dependencies and their
required x/sys and x/text versions. References:
[GO-2026-6888](https://pkg.go.dev/vuln/GO-2026-6888),
[GO-2026-6865](https://pkg.go.dev/vuln/GO-2026-6865).
The source fallback retains Go's checksum database and checks the complete local
compiler version against the required patch version. Release automation refuses
unsigned publication; the signing seed is held outside this repository.

## خلاصهٔ فارسی

<div dir="rtl">

این فورک با نام Ravan روی نسخهٔ منتشرشدهٔ۱٫۸٫۵ ساخته شده است. اصلاحات بازیابی
نشست و شمارنده‌ها از بالادست برگشته‌اند؛ تشخیص کانال یک‌طرفهٔ WSS نیز اضافه شده
تا بازبودن سوکت به‌اشتباه نشانهٔ سلامت نباشد. تونل بیکارِ سالم فقط به دلیل نبود
ترافیک ریستارت نمی‌شود. فایل‌ها و سرویس‌ها از نسخهٔ اصلی جدا هستند. ابتدا روی
پورت‌های آزاد هر دو سمت تست کنید؛ قطعی مسیر اینترنت با این تغییرات درمان نمی‌شود.
علت انحصاری قطعی گزارش‌شده روی سرور واقعی هنوز اثبات نشده و آزمون چندساعتهٔ همان
مسیر لازم است. کد اتصال و تنظیمات حاوی اطلاعات دسترسی را عمومی نکنید.

</div>
