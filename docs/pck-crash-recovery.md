# PCK client crash recovery and probe diagnostics

Last verified against Ravan v0.1.7.

## Client change

The v0.1.5 server sends an existing RTT request every five seconds. Its client
answered those requests but learned its silence deadline only from ten-second
heartbeats. Consequently a server crash could still cost roughly thirty seconds
before redial. The raw PCK client now learns the RTT-request cadence separately.
It requires three observed gaps before using that cadence, retains the
fifteen-second liveness floor and tolerates three intervals of silence.
Heartbeat and probe timestamps are never mixed: coincident signals are not proof
of a faster sender. A legacy server's single startup probe cannot shorten the
deadline. Other KCP carriers retain their existing policy. The learned clock is
new for each control generation, so a replacement server can be slower.

Updating the **outside/client side** enables this change; update both endpoints
so that the five-second requests added by the v0.1.5 server are available.
The wire format, authentication and existing configuration are unchanged.
Active connections may break during recovery; this does not promise seamless
failover or eliminate outages outside the tunnel's control.

## The four earlier failed probes

The v0.1.5 hour recorded two failed probes per mode outside injected-fault
windows plus sixty seconds of recovery allowance. Its driver discarded the
exception details. Some samples also had multi-second collection gaps. Those
files cannot establish whether every failure was path loss, a delayed mux stream
or local scheduling delay, so no exclusive retrospective cause is asserted.

The diagnostic workload now retains each failed probe's connection/send/receive
phase, exception category, wall and monotonic elapsed time, and local scheduling
lag. Periodic telemetry retains cgroup CPU throttling/pressure. A bounded,
peer-and-test-port-filtered header capture is private diagnostic evidence only.
Probe timeout remains two seconds; failures are not erased by retries or a
longer timeout. Startup and deliberate faults are counted separately from quiet
windows. Test harness behavior is distinct from production tunnel behavior.

## Measurements — 18 November 2026

In the namespace comparison, the same v0.1.5 server and server-crash schedule
were used with old and new clients. Application recovery was **35.0 seconds**
with the old client and **18.5 seconds** with the candidate. WSS Mux was about
6.9 seconds in both. These are short emulated Linux comparisons, not hardware
throughput figures. A separate 60-second old-v0.1.4-server/new-client run retained
one PCK control claim and ended with intact data, checking the legacy timing path.

The real Iran/outside comparison kept the complete experiment group at 10% of
one core per host, 512 MiB, and about 1.05 Mbit/s aggregate offered payload.
Only owned test engine processes were crashed; existing services were preserved.
Recovery below is from the actual stop/filter installation to a successful probe
after an observed failure and includes the intentional outage itself.

| Fault | PCK application recovery | WSS Mux application recovery | PCK control redial |
|---|---:|---:|---:|
| server_crash 1 | 16.0 s | 7.3 s | 15.2 s |
| server_crash 2 | 16.9 s | 7.2 s | 15.3 s |
| server_crash 3 | 16.3 s | 7.7 s | 15.0 s |
| return 4 | 18.7 s | 23.7 s | 17.0 s |

One quiet WSS Mux timeout was reproduced before the resource-profile correction:
the read operation reported timeout after 7670.6 ms; nearby scheduler-watch lag
was 7694.0 ms and PCK's sample gap was 7992.1 ms. Cgroup full CPU-pressure time
grew by about 7.21 seconds across a 10.05-second interval. No control redial
occurred at that incident. These observations establish local scheduling starvation
as a contributor to this reproduced timeout; a failed echo alone cannot establish
packet loss or a broken tunnel. The old four failures lack this instrumentation,
so an exclusive retrospective explanation of every old failure remains unproven.

Shortening only the experiment's CPU quota period from 100 ms to 10 ms did not
remove multi-second scheduling pauses. Later host-health samples recorded
16.88 and 19.11 aggregate CPU-seconds of steal across 9.18 and 9.55-second
intervals, respectively, alongside local wake-up delays above seven seconds.
Steal is time the virtual CPUs did not receive from the hypervisor. This is
evidence of a host-resource contribution to these later pauses, not proof that
every historical failure had the same exclusive cause. Guest tunnel code cannot
guarantee removal of hypervisor scheduling pauses.

The final lab phase also restricted each experiment group to one vCPU, retaining
the short quota period. `cpu.max` stayed `1000 10000`: the total budget remained
10% of one core, with unchanged offered load and the same two-second per-operation
probe timeout. Production service quotas and affinity were not modified. These
are lab-profile changes, separate from the PCK client recovery fix; they are
not installed as production defaults or advertised as a cure for host overselling.

- **PCK:** 140,255,232 bidirectional payload bytes; zero corrupt payloads. Quiet-window failed probes in the original/short-period/single-vCPU phases: 0/0/5. Quiet-window maximum scheduling-watch lag: 3396.9/8779.9/9669.4 ms. The final single-vCPU observation lasted 900 seconds without another injected fault.
- **WSS Mux:** 139,284,480 bidirectional payload bytes; zero corrupt payloads. Quiet-window failed probes in the original/short-period/single-vCPU phases: 1/0/0. Quiet-window maximum scheduling-watch lag: 7694.0/3561.6/10199.4 ms. The final single-vCPU observation lasted 900 seconds without another injected fault.

Quiet windows exclude each injected fault and sixty seconds after removal;
all failures, including deliberate faults and startup, remain in the raw counters.
Neither quota-period tuning nor the single-vCPU trial eliminated the long
scheduling pauses. **The fast crash-recovery target was met; the zero-quiet-timeout
target was not met.** These trials are evidence, not newly certified production
defaults.

Paired private PCK header captures were compared around the three shorter
receive timeouts. Of 1227 unique Iran-to-outside frames observed at the sender,
1081 had matching receiver frames and 146 did not (about 11.9%). The outside
capture reported zero kernel capture drops; ignoring IPv4 identification in
the comparison did not recover those matches. The preceding window also had
178 unmatched frames out of 1583, indicating an ongoing delivery impairment,
not just a single application timeout. Sender-side capture precedes some local
egress stages, so this does not locate the loss exclusively inside the ISP.
Reverse-direction comparisons are less conclusive because the Iran capture
reported 54 kernel capture drops across the complete capture.
These observations support both host-scheduling and delivery-loss contributions;
the relative cause of each old failed probe cannot be reconstructed from the old
logs. No claim is made that the client-clock fix removes either external condition.

No one-hour claim is made for this new run. The previous v0.1.5 hour is a separate
historical run. A short run without another timeout does not prove that every
ISP path is lossless or that future production outages have been eliminated.

Linux validation passed the complete client transport race package (36.001 s),
focused clock/control tests, staticcheck, vet, documentation/attribution checks,
legacy-server compatibility, and the native crash comparison. Release binaries
are separately installed and smoke-tested after rebuilding with final metadata.
The real run used candidate SHA-256
`1c669b2d9dd6df5b01c098a1737393a1ca9925b2587264b42d4654b3b10bbf11`.
Its label preceded the v0.1.6 metadata bump; client transport code was frozen
during this run. arm64 is cross-built, not hardware-tested.

See [compact measurements](pck-crash-validation.json). The release's signed
checksum manifest covers `ravan-pck-crash-evidence.json.gz`, with sanitized
timestamped samples, probe-error categories and CPU telemetry. Private header
captures, addresses, pairing tokens and TLS material are not published.

## Reproduction and limits

The opt-in namespace helper supports `--fault-kind server_crash` and
`--fault-offset 30` for a warmed-up short comparison. Set `--client-binary` to
compare clients against the same server binary. Keep network and mount isolation
enabled. The regular hour-long schedule is unchanged. Linux root or the required
namespace/raw-socket/netem capabilities are necessary.

## خلاصهٔ فارسی

<div dir="rtl">

کلاینت PCK اکنون ریتم پیام‌های سلامتِ دوره‌ای را جدا از heartbeat یاد می‌گیرد تا
پس از کرش ایران زودتر اتصال را دوباره بسازد. یک پیام ابتدایی از نسخهٔ قدیمی برای
کوتاه کردن انتظار کافی نیست. برای خطاهای گذرای تست نیز جزئیات ثبت می‌شود؛ لاگ قبلی
برای اثبات علت دقیق همهٔ آن‌ها کافی نبود. مهلت تست افزایش نیافته و خطاها پنهان نمی‌شوند.

</div>
