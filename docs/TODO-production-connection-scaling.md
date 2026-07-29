# TODO — Production connection scaling (infra side)

Companion to branch `feat/production-connection-scaling`. The Go-side changes
are done and tested; everything listed here lives **outside this repo** (the
`k8s/` repo, node configuration, or a load-testing exercise) and still needs
doing before the server is trusted with a high connection count.

Context: the WebSocket layer (`core/service/websocket/server/gobwas`) uses
epoll via `mailru/easygo/netpoll` with a shared worker pool rather than two
goroutines per connection. That makes it cheap in memory but means **file
descriptors, not RAM, are the scarce resource**, and the ceiling is set by
limits the process does not control on its own.

---

## What already landed in this repo (no action needed)

| Change | Where |
| --- | --- |
| `GOMAXPROCS` from cgroup CPU quota + soft `RLIMIT_NOFILE` raised to hard, both logged at startup | `pkg/runtimetune`, called from `cmd/pipewave-server/main.go` |
| Worker count from `GOMAXPROCS` (was `runtime.NumCPU()`, which ignores the CPU limit) with a floor of 8 | `provider/worker-pool-provider` |
| Queue depth default 64 → 4096; thresholds derived as 3/4 and 1/4 of it | `export/types/config_child.go` |
| `ACTIVE_CONNECTION.MAX_CONNECTIONS` cap, enforced with an exact CAS reservation | `core/service/websocket/server/gobwas` |
| Rejected-connection and worker-queue counters in the stats log | `PrintStats` |

---

## 1. Raise the pod CPU/memory limits — **blocking**

`k8s/base-app/01-deployment.yaml` currently has:

```yaml
resources:
  limits:
    cpu: 500m
    memory: 1000Mi
  requests:
    cpu: 2m        # ← 2 milli-core
    memory: 50Mi
```

`requests.cpu: 2m` is the problem for scheduling: the scheduler places the pod
as if it needs almost nothing, so it lands on packed nodes and gets starved
under contention. It also makes the HPA's CPU-utilisation target meaningless —
utilisation is computed against *requests*, so actual usage sits at thousands
of percent and the HPA scales on noise.

Suggested starting point, to be corrected by the load test in §5:

```yaml
resources:
  limits:
    cpu: "2"
    memory: 2Gi
  requests:
    cpu: "1"       # keep close to limits for a latency-sensitive server
    memory: 1Gi
```

Note `runtimetune` now derives `GOMAXPROCS` from `limits.cpu`, so a fractional
limit (`500m` → `GOMAXPROCS=1`) directly caps parallelism. Prefer whole cores.

## 2. Let the HPA actually scale — **blocking**

Same file:

```yaml
minReplicas: 1
maxReplicas: 1     # ← scaling is disabled
```

With `maxReplicas: 1` every connection lands on one pod and one fd table, and
the connection cap from §4 turns into a hard service ceiling. Raise
`maxReplicas` once §1 is in.

Two caveats specific to long-lived WebSockets:

- **CPU/memory utilisation is a weak signal here.** A pod can hold its fd
  ceiling worth of idle connections at low CPU. Consider scaling on
  `pipewave_connections_active` (already exported) via a custom/external
  metric instead of, or alongside, CPU.
- **Scale-down evicts connections.** `terminationGracePeriodSeconds: 60` plus
  the existing graceful shutdown covers draining, but the default HPA
  scale-down policy can still churn. The current 120s stabilisation window is
  reasonable; revisit if reconnect storms show up.

## 3. File descriptor limit — verify, then pin

`runtimetune` raises the **soft** limit to the **hard** limit and logs both. It
cannot raise the hard limit (unprivileged), so the hard limit set by the
container runtime is the real ceiling.

Action: after deploying, check the startup log line and confirm the value is
what you expect:

```
runtimetune: applied gomaxprocs=2 num_cpu=8 fd_soft_limit=1048576 fd_hard_limit=1048576
```

Kubernetes has **no per-container ulimit field**, so if that number is too low
the fix is at the node level — `LimitNOFILE` in the containerd/kubelet systemd
unit, or the node image/launch template (for EKS managed node groups, via
user-data). Do not add a privileged initContainer just for this.

Also watch for this warning, which means the CPU quota was not detected and
§1's limit is being ignored:

```
runtimetune: GOMAXPROCS equals host CPU count — if this pod has a CPU limit, ...
```

## 4. Set `MAX_CONNECTIONS` once the real ceiling is known

Ships as `0` (disabled) because a wrong value is worse than none — too low and
you reject traffic the pod could serve. Set it in the ConfigMap
(`k8s/base-app/configs/config.yaml`) after §5:

```yaml
ACTIVE_CONNECTION:
  MAX_CONNECTIONS: 50000   # example only — derive from the load test
```

Size it below `fd_hard_limit` with headroom for the Postgres pool
(`MAX_CONNS: 15`), Valkey, DynamoDB keep-alives, and log files. Rejected
connections surface as `ErrServerAtCapacity` and increment
`connections_rejected_total`; sustained growth there means the cap is binding
and the deployment needs more replicas, not a bigger cap.

## 5. Load test to find the real numbers — **prerequisite for §1 and §4**

Every number above is a starting guess. Per-pod capacity depends heavily on
frame size and message rate, which cannot be derived from the code. Measure:

- connections held before latency degrades,
- `worker_tasks_dropped_total` — must stay flat; any sustained growth means
  `BUFFER` or the worker count is too small for the load,
- `worker_queue_length` vs `worker_queue_capacity` at peak,
- memory per connection (already in the stats log as `memory_per_connection_kb`),
- behaviour during a mass reconnect (kill a pod with N connections held).

## 6. Kernel / network limits — check before the first real traffic peak

None of these are set anywhere today, and each bites *before* the fd limit
does. They are node-level, not pod-level.

- **`net.core.somaxconn`** — accept-queue depth. A mass reconnect (deploy,
  network blip) overflows it and clients see connection timeouts rather than
  errors. This is usually the real culprit behind "connections fail under
  load", not fd exhaustion.
- **`nf_conntrack_max`** — with kube-proxy in iptables mode, every long-lived
  WebSocket holds a conntrack entry. Overflow shows up as random packet drops
  and `nf_conntrack: table full, dropping packet` in `dmesg` — very hard to
  diagnose from inside the app. Check alongside
  `nf_conntrack_tcp_timeout_established`.
- **Ephemeral ports at the gateway** — traffic arrives via the Gateway
  (`k8s/staging/routes/00-gateway.yaml`). The gateway→pod hop is limited to
  ~28k connections per (src IP, dst IP, dst port) tuple. **This is the most
  common hard ceiling in Kubernetes and no amount of ulimit tuning moves it.**
  The fix is more pods (§2), more ports, or a wider
  `net.ipv4.ip_local_port_range`.

## 7. Optional: pool frame buffers

`processClientMessage` allocates a fresh `make([]byte, header.Length)` per
frame, with `MaxFrameSize` at 1 MB. At high message rates that is real GC
pressure. `gobwas/pool` is already an indirect dependency. Worth doing only if
the load test shows GC as a bottleneck — not before.
