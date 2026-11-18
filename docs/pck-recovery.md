# PCK recovery validation

Last verified against Ravan v0.1.4.

Reviewed for the link-prefix-only v0.1.4 update; historical test measurements below are unchanged.

## Change

The dialler used any authenticated incoming packet as evidence of liveness.
With only its outgoing PCK flow blocked, incoming data or MTU probes could
continually reset the silence clock. The next routine rekey finally noticed
the failure, then the carrier kept trying the same blocked source port for
another 90 seconds. The v0.1.2 validation measured 156.2 seconds of downtime.

PCK and its SNI wrapper now check an authenticated round trip every five
seconds, including when idle and with `auto_mtu = false`. Three unanswered
probes are followed by a handshake round. If a previously established flow
still cannot complete that handshake, the generation ends immediately and
the existing restart loop opens a carrier with a new source port. Ordinary
startup failures keep the existing retry policy. UDP, QUIC and reverse KCP
policies are unchanged.

Probe support comes from authenticated v2 negotiation or an authenticated
answer to our own probe. Pre-probe peers keep the legacy behavior. No new
message kinds, cryptographic changes, credentials or configuration keys are
introduced. Install v0.1.3 on the dialling side to obtain the recovery change;
updating both ends is recommended. This does not make a permanently blocked
destination port or a fully unavailable route reachable.

## Validation

Measurements and test limitations are recorded in
[the validation record](pck-recovery.json). Tests use Debian 12 Linux in a
local QEMU/TCG VM with two virtual CPUs. Real TUN devices and raw carriers
run between isolated network namespaces, not across the public Internet.
The flow filter remains installed: successful recovery must use a different
source port, carry a ping, and transfer a byte-identical 5 MiB payload.

The regression test also keeps authenticated incoming traffic flowing while
dropping every outgoing packet. It runs with AutoMTU disabled. The old PCK
policy fails its deadline; the new policy passes. A healthy idle peer must
not be rekeyed merely because round-trip monitoring runs.

| Scenario | Observed recovery |
| --- | ---: |
| PCK outgoing flow blocked, earlier validation | 156.2 s |
| PCK outgoing flow blocked, corrected build | 17.65 s / 19.03 s |
| PCK return flow blocked | 20.75 s / 19.58 s |
| PCK listener killed and restarted | 8.07 s / 8.98 s |
| SNI-wrapped PCK outgoing flow blocked | 18.13 s / 20.39 s |

All recovered flows carried byte-identical 5 MiB transfers. The live matrix
covers UDP (one/two paths), QUIC, PCK, SNI, XDI, Spoof, listener restarts,
blocked flows, 20 ms/1% netem paths, and six v0.1.2/current endpoint pairs.
SNI initially hid its random source port through the wrapper: the harness
selected the wrong flow via its legacy fallback. The wrapper now forwards
the existing carrier diagnostics. Correct-port SNI reruns passed twice.
The initial failed run is retained: coverage is 21 unique scenarios across
the matrix and corrected rerun, not one all-green matrix invocation.

The recovery/probe race suite and final SNI/recovery race suite pass without
reported races. An unchanged capped-path AutoMTU test still fails its
20-second gate under race instrumentation in TCG, in both the candidate and
the old v0.1.2 source. Ordinary old/new runs pass in 1.35/1.37 seconds. Failed
runs are retained and no deadline or assertion was weakened. This is not a
whole-repository race certification.

Ordinary L3 functional checks run from the package source directory. The
absolute nanosecond packet-path budget gate is excluded in TCG; its initial
failure is retained. A first standalone invocation also lacked the correct
working directory for a source-reading test; the corrected run uses the
package directory. No functional assertion or timing limit was relaxed.

No new multi-hour soak, public Iran-to-Turkey route acceptance, or ARM64
hardware execution is claimed. The 35-second test threshold is a lab
acceptance criterion, not a guarantee under arbitrary CPU or network load.
In particular, a full route outage can persist beyond that threshold until
the route itself becomes usable again.

## Reproduction

```sh
go test -race ./internal/tunnel/l3 -run 'TestPCKDetects|TestADiallerRecovers|TestSilence|TestAProbe|TestProbeBody|TestAnUnanswered|TestSpoofIs' -v
BP_L3_LIVE=1 go test ./internal/e2e -run TestL3CarriersOverARealTUN -v -timeout 30m
```

The live tests need Linux namespace, TUN, raw-socket and traffic-control
support. Never run the shell harness directly in the host's namespaces.
Mixed-version runs additionally set `BACKPACK_PREV_BINARY` to v0.1.2.

## خلاصهٔ فارسی

<div dir="rtl">

خرابی یک‌طرفهٔ مسیر رفت قبلاً ممکن بود با رسیدن ترافیک از سمت مقابل پنهان بماند.
نسخهٔ جدید رفت‌وبرگشت PCK را بررسی می‌کند و پس از تأیید خرابی، پورت مبدأ تازه‌ای می‌سازد.
تست‌ها روی تونل واقعی در شبکهٔ محلیِ جدا انجام می‌شوند؛ نتیجه تضمین زمان اتصال روی
اینترنت یا هنگام قطع کامل مسیر نیست. برای استفاده از اصلاح، سمت آغازکننده را به‌روز کنید؛
به‌روزرسانی هر دو طرف توصیه می‌شود. تنظیمات دستی و کلیدها تغییر نمی‌کنند.

</div>
