# PCK FEC pairing and validation

Last verified against Ravan v0.1.7.

## Pairing fix

PCK and xDi use KCP's FEC settings too, but reverse setup-link export and
import previously treated only the transport named `kcp` as carrying that
pair. A custom pair could be dropped, and explicit FEC off could become
the peer's preset pair. This can produce incompatible endpoints even when
their token and transport match. It is separate from the transient failures
observed between explicitly matched configurations in the network trial.

New links preserve the exact data/parity pair for all three KCP-based
transports, whether exported from Iran or outside. An appended format-2
field distinguishes explicit FEC off from an older PCK/xDi link that omitted
the pair. Older PCK/xDi links retain their preset fallback; they cannot
recover custom FEC values that were never encoded. Regenerate those links
from the actual configuration. Update both endpoints before using a newly
generated link; older readers reject its new field and request an update.

The Linux regression reproduced the old failure for PCK and xDi, including
zero/zero. Cases cover off, 2:1 and 10:3 through the wizard, managed setup
and export from either endpoint, plus legacy preset fallback. The short
management package, its short race suite, vet and staticcheck passed.
The runtime, operation deadlines and retry policy are unchanged from v0.1.6.

## Separate FEC experiment

The three-way real-path trial used the published v0.1.6 binary with explicitly
matched settings: PCK without FEC, PCK FEC 2:1, and WSS Mux. Each experiment
group stayed under a 10%-of-one-core CPU quota, 256 MiB memory cap and a
2 Mbit/s additional-load authorization. Aggregate offered bidirectional
application payload was 0.393216 Mbit/s; that excludes protocol overhead,
parity and retransmissions. Each mode had two persistent workers and
two fresh-connection probes per second, with the same two-second operation
timeout. Startup counters are excluded from the measurement window.

For five minutes only, owned egress filters selected the two test PCK ports
to drop packets whose IPv4 ID's lowest three bits were zero. This selected
every eighth ID; it was deterministic, not random independent loss.
WSS Mux was not subjected to that added filter. Neither the root qdisc nor
production-service configuration was changed.

The measured window lasted 1200.50 seconds. Results exclude warmup:

| Mode | Successful fresh probes | Failed fresh probes | Persistent-worker errors | Corrupt payloads |
| --- | ---: | ---: | ---: | ---: |
| PCK, FEC off | 2220 | 3 | 6 | 0 |
| PCK, FEC 2:1 | 2221 | 4 | 6 | 0 |
| WSS Mux | 2210 | 6 | 10 | 0 |

Persistent-worker errors and probe failures are separate counters, not a
single failure total. PCK's three failures split 2 before and 1 during the
added loss; FEC's four split 1 before, 1 during and 2 after. WSS Mux had
two in each phase despite not receiving the added filter. All modes carried
traffic again without a service restart. This trial did not pass the
zero-timeout criterion and does not establish that FEC fixes the remaining
production symptom.

The Iran experiment averaged 9.77% of one core over its resource sample span,
close to its deliberately strict 10% cap; 10,378 of 12,180 cgroup periods
were throttled. Host telemetry also recorded up to 21.71 aggregate stolen
CPU seconds across eight CPUs within a 7.61-second sample interval. This
number is aggregate CPU time, not a 21.71-second wall pause. The application
sampler recorded gaps up to 10.49 seconds. Outside averaged 3.18% of one core.
Neither group recorded an OOM. Quota saturation and host scheduling make
this an unsuitable basis for predicting uncapped production throughput or
uniquely attributing every timeout to the network. No quota, affinity or
timeout adjustment is installed as a production default.

See [compact measurements](pck-fec-validation.json). Addresses, tokens,
TLS material and private logs are not included. The owned test units,
filters, raw-PCK guard rules and test directory were removed after collection.

A separate 180-second Linux namespace trial combined the same loss selector
with an eight-second stop/resume of the two owned server processes. After
warmup, each PCK variant had four failed fresh probes and zero corrupt
payloads. Persistent-worker errors were 8 without FEC and 7 with 2:1.
During the loss phase, median sampled latency was about 108 ms versus
61 ms. Both variants returned to normal after the faults. These observations
support a latency benefit in that particular experiment, not a cure for
scheduling pauses or a universal reliability guarantee.

## خلاصهٔ فارسی

<div dir="rtl">

لینک نصب اکنون تنظیم دقیق FEC را برای PCK و xDi هم منتقل می‌کند، حتی وقتی خاموش
باشد. هر دو طرف را به‌روز کنید و برای تنظیمات سفارشی، لینک تازه بگیرید. لینک قدیمی
اطلاعاتی را که قبلاً حذف شده بازیابی نمی‌کند. آزمایش FEC، وقفه‌ها را کاملاً حذف
نکرد؛ این نسخه اصلاح جفت‌سازی است و تضمین اتصال بدون وقفه نیست.

</div>
