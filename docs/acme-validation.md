# Certificate validation

Last verified against Ravan v0.1.7.

Reviewed for v0.1.7. Historical measurements below are unchanged; Reverse PCK recovery is validated separately.

The live certificate results below were collected on v0.1.1 and retain that version
in their JSON record. The certificate implementation is unchanged in v0.1.2; this
page was reviewed without claiming another public-CA issuance run. v0.1.3 changes
PCK recovery only; no new certificate issuance is claimed.

## Fixed failures

The HTTP-01 handler used to be registered only when the first HTTP request arrived.
Go autocert enables HTTP-01 when its HTTPHandler is constructed, so the initial
order could try only TLS-ALPN-01. With the tunnel off port 443, issuance failed
with "no viable challenge type found". The handler is now registered before
startup issuance and on every manager replacement.

PEM renewal now tracks both certificate and key file identity, size and timestamp.
Atomic replacements with equal or older timestamps reload; incomplete/mismatched
updates retain the last good pair and are retried on the next handshake. The menu
validates that PEM files match and saves absolute paths for systemd. Panel fallback
logging uses sync.Once so concurrent handshakes do not race. Rate-limit errors
are returned immediately instead of being retried behind a handshake timeout;
transient failures retain exponential retries and honor Retry-After. ACME tunnels still
require a valid issued certificate; they do not silently fall back to self-signed.

## Reproducible checks

The controlled ACME fixture is adapted from the Go x/crypto v0.59.0 tests with
its BSD license preserved in internal/testacme. It exercises actual order,
authorization, challenge, signing, cache and renewal flows; it is not a public CA.
Tests verify HTTP-01 and TLS-ALPN-01, trusted chains, domain SNI, absent SNI,
rejection of other names, cache loading with an offline CA, accelerated automatic
renewal, partial PEM rotation and concurrent fallback. Normal builds have no
custom CA override and do not link the fixture.

The live transport test requires the acmelive build tag and explicit domain,
private cache directory and WSS/WSSmux mode environment variables. It verifies
system trust and hostname even in the absent-SNI probe, then sends four payloads
through the actual client/server transport. Ordinary tests never opt into public
certificate issuance. See [the validation record](acme-validation.json) for observed live results.

This reproduces a failure class in this fork; without logs and the exact installed
upstream version, it does not prove the exclusive cause of the historical BackPack report.

## Live outcome and limits

A production Let's Encrypt certificate was issued after the authority's Retry-After
using a separate HTTP-01-only diagnostic harness and the same account/cache. WSS
and WSSmux then each passed four 69,632-byte round trips with that certificate,
including system-trust/hostname verification, absent SNI and wrong-name rejection.
The same leaf fingerprint was served after the process changed from WSS to WSSmux.
The public certificate was revoked after testing, as was the staging certificate.
All temporary server files, keys, processes, units and timer were removed. Existing
TCP 443 service and engine were preserved. Certificate transparency records remain
public even after revocation and server cleanup.

This is not a claim that the native manager completed a fresh public-CA challenge
on that host: its earlier attempts met authorization rate limits while another
service occupied TCP 443. Fresh issuance and accelerated renewal through the native
manager passed with the controlled CA. ARM64 was cross-built/scanned, not run.
Targeted certificate race checks passed. Broader invocations retained an allocation
assertion failure (including under race instrumentation) and a large setup-link
matrix timeout; no entirely green full-suite result is claimed. See the JSON record.

## Operational requirements

Keep /etc/ravan/acme persistent and private. Point the domain's DNS to the server;
allow public inbound TCP 80 for HTTP-01, or place the TLS listener on TCP 443 for
TLS-ALPN-01. An unrelated service on 443 can delay fallback to HTTP validation.
A menu restart acknowledgement means configuration was saved, not that issuance
succeeded: check the tunnel's journal for "certificate ... is ready". Generated
self-signed PEM files are separate from autocert's issued-certificate cache.

## خلاصهٔ فارسی

<div dir="rtl">

فعال‌سازی تأیید دامنه روی پورت ۸۰، بارگذاری فایل‌های تمدیدشده و بررسی تطابق کلید اصلاح شد.
گواهی صادرشده در حافظهٔ پایدار ACME نگهداری می‌شود و با فایل خودامضا فرق دارد.
پیام ذخیرهٔ تنظیمات به معنی موفقیت صدور نیست؛ آماده‌شدن گواهی را در گزارش سرویس بررسی کنید.
تمدید در آزمون کنترل‌شده با عمر کوتاه آزمایش می‌شود؛ نتیجهٔ صدور عمومی جداگانه ثبت شده است.

</div>
