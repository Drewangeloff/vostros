# Old School Bird — Architecture

An infinitely scalable clone of old-school Twitter (2006–2012 era), fully operated by AI agents.

---

## Current Plan: v0.4

> **Last updated:** 2026-03-02
> **Status:** Building
> **Summary:** Neon Postgres from day one, multi-instance Cloud Run, CSRF protection, clean route namespacing

[Jump to current plan →](#v04--neon-postgres-from-day-one-actually-scalable)

---

## Version History

| Version | Date | Summary |
|---------|------|---------|
| v0.1 | 2026-03-02 | Initial architecture — managed services, one part-time human |
| v0.2 | 2026-03-02 | Fully agent-operated, zero-cost-at-zero-users, "all open source" on Cloud Run |
| v0.3 | 2026-03-02 | Hardened v0.2: SQLite early stage, sync moderation, async fan-out, realistic costs, infra safety nets |
| v0.4 | 2026-03-02 | **CURRENT** — Drop SQLite/Litestream, Neon Postgres from day one, multi-instance, CSRF, /u/ routes |

---
---

## v0.4 — Neon Postgres from Day One, Actually Scalable

> **THIS IS THE CURRENT PLAN**

### Design Goals

1. **$0/mo at zero users** (true scale-to-zero)
2. **Fully agent-operated** — no human in the loop at steady state
3. **Open-source-core with serverless managed services** — application code is fully open source and portable; we use managed services (Neon, Upstash) for serverless pricing
4. **Runs on Cloud Run** (serverless containers, scale to zero, pay-per-request)
5. **Multi-instance from day one** — actually scalable, not artificially limited
6. **No content goes live without passing moderation** — synchronous safety in the write path
7. **Infrastructure safety nets** — budget caps, instance limits, dead man's switches
8. **Go + HTMX frontend** — server-rendered HTML, HTMX for interactivity, JSON REST API for agents

### What Changed from v0.3

| Problem in v0.3 | Fix in v0.4 |
|------------------|-------------|
| SQLite + Litestream forces max-instances=1 (not scalable) | Neon Postgres from day one — multi-instance Cloud Run |
| Litestream adds operational complexity (restore on cold start, async replication window, GCS config) | Eliminated — Neon handles persistence, replication, backups |
| SQLite → Postgres migration required at ~1K users (data export, FTS5→tsvector, SQL dialect changes) | No migration needed — start on Postgres, stay on Postgres |
| FTS5 search requires separate migration to tsvector later | Postgres tsvector from day one |
| No CSRF protection for cookie-based auth | CSRF token middleware (per-session token in meta tag, HTMX sends via hx-headers) |
| `GET /{username}` route conflicts with `/timeline`, `/global`, etc. | User profiles at `/u/{username}` — clean namespacing |
| Frontend stack unresolved (listed as open question) | Decided: Go templates + HTMX |
| No dev mode for templates | `-dev` flag reads templates from disk with hot-reload |

### What's Retained from v0.3

Everything else from v0.3 carries forward unchanged:
- Async fan-out via outbox pattern
- Sync moderation in write path (regex → local classifier → Haiku)
- Upstash Redis from day one for rate limiting
- Agent architecture with safety nets (Auditor, dead man's switch, action caps)
- Cloudflare free tier for CDN/edge
- GCP observability (Cloud Logging/Monitoring free tier)
- Infrastructure circuit breakers (max instances, budget alerts)
- Soft-delete everywhere

### What Changed from v0.2

| Problem in v0.2 | Fix in v0.3 |
|------------------|-------------|
| "100% open source" was a lie (Neon, Upstash are SaaS) | Honest framing: open-source-core + managed services. SQLite/Litestream for Phase 0-1 if you want true self-hosting |
| LLM costs wildly underestimated ($300 at 1M users) | Realistic tiered model: regex first, Haiku second, Sonnet for edge cases. ~$800-1,500/mo at 1M users |
| Cold starts (Cloud Run + Neon = 2-4s on first request) | Connection pooling via PgBouncer sidecar; min-instances=1 once you have paying users (pennies/mo) |
| Meilisearch on Cloud Run doesn't survive scale-to-zero | Dropped. Use Postgres `tsvector` until Phase 3, then dedicated Meilisearch on GCE/Fly |
| Fan-out-on-write was inline (blocks the request) | Async from day one: write tweet → return 200 → goroutine processes fan-out via outbox |
| In-memory rate limiting doesn't survive scale-to-zero | Upstash Redis from day one ($0 at zero requests, needed for rate limiting) |
| Agent event triggers were hand-wavy | Concrete: inline moderation in write path, outbox polling for async agents, Cloud Scheduler for cron agents |
| No one audits the Auditor | Dead man's switch: Cloud Scheduler pings a healthcheck URL; if Auditor misses 2 runs, alert fires to fallback webhook |
| No observability story | Cloud Logging + Cloud Monitoring (GCP free tier) + Ops agent consumes GCP metrics API |
| No infra abuse protection | Cloud Run max-instances cap, GCP budget alerts, Cloudflare rate limiting rules (free tier) |

---

### Compute — Cloud Run

Every service is a container image deployed to Cloud Run:
- Scale to zero when idle (Phase 0)
- Scale horizontally under load
- **Max instances cap** to prevent runaway costs (e.g., max 10 at Phase 1, increase as needed)
- Pay only for request duration
- Optional: set `min-instances=1` once you have real users to eliminate cold starts (~$5/mo)

---

### Database — Neon Postgres (All Phases)

- **Neon Serverless Postgres** from day one
- **Scales to zero** — $0 when nobody is querying
- Uses Neon's built-in **connection pooler** (PgBouncer) — handles Cloud Run's bursty connection patterns on cold start
- Postgres ecosystem from the start: `tsvector` for search, `LISTEN/NOTIFY` for events, `jsonb` for flexibility
- Branches for dev/staging are free
- No migration needed — start on Postgres, stay on Postgres

#### Phase 0-1 (0–10K users): Neon free tier
- 0.5 GB storage, autoscaling compute
- $0 at zero queries

#### Phase 2 (10K–100K users): Neon Pro
- More compute, more storage
- Add read replicas for timeline queries

#### Phase 3+ (100K+ users): Neon Scale
- Dedicated compute for write-heavy workloads
- Multiple read replicas

> **Why not SQLite + Litestream?** SQLite forces `max-instances=1` on Cloud Run (single-writer). Litestream adds cold start latency (restore from GCS), an async replication window (potential data loss), and operational complexity. The SQLite → Postgres migration is a significant effort, not a config toggle. Neon gives us $0 at zero usage AND multi-instance scalability from day one.

**Go driver:** `github.com/jackc/pgx/v5` — the standard Postgres driver for Go. Connects via Neon's pooler endpoint.

---

### Timeline Cache — Upstash Redis (from Day One)

- Serverless Redis with per-request pricing
- **$0 at zero requests** — this is why we include it from day one
- Used for:
  - **Rate limiting** (critical — can't rely on in-memory with scale-to-zero)
  - **Session cache** (faster than DB lookup)
  - **Timeline sorted sets** (Phase 2+, when timeline queries outgrow Postgres)
- At Phase 0-1: primarily rate limiting + sessions. Timelines served directly from SQLite/Postgres.

> **Why from day one?** Because rate limiting without persistent state is no rate limiting at all. Upstash at zero requests costs $0, so there's no reason to defer it.

---

### Event Streaming — Async Outbox Pattern

#### All Phases: Outbox Table + Async Processing

```
User posts tweet
  → API validates + runs sync moderation (see below)
  → Writes tweet + outbox event to DB in one transaction
  → Returns 200 to user immediately
  → Background: goroutine / cron reads outbox, processes fan-out
```

**Phase 0-1:** A goroutine in the API process polls the outbox table every 1-2 seconds. Simple, no external dependencies.

**Phase 2+:** Replace outbox polling with **Upstash Kafka** (serverless, per-message pricing). The outbox table becomes the Kafka producer. Fan-out consumers scale independently.

> **Why not LISTEN/NOTIFY?** It doesn't survive Cloud Run scale-to-zero (no persistent connection to receive notifications). The outbox pattern works regardless of whether the process was just cold-started.

---

### Search — Postgres tsvector (All Phases, Until You Can't)

- Postgres full-text search via `tsvector` / `tsquery` from day one
- Good enough for old-school Twitter's simple text search
- Zero additional infrastructure — it's just a column + GIN index
- Auto-updated via a generated column or trigger
- **Phase 3+** (if needed): Stand up Meilisearch on a dedicated VM (GCE e2-micro = ~$7/mo) for richer search features (typo tolerance, faceting)

> **Why not Meilisearch on Cloud Run?** Cloud Run volumes don't survive scale-to-zero. You'd re-index from scratch on every cold start. Not viable.

---

### Blob Storage — Cloud Storage (GCS)

- Profile pictures, media attachments (if we add them)
- Pennies per GB, $0 at zero storage
- Serve via Cloudflare CDN (free tier) with GCS as origin
- Cost: effectively $0 until meaningful media volume

---

### Auth — JWT + bcrypt, Self-Contained

- **argon2id** (preferred) or bcrypt for password hashing, in the Go service
- Short-lived JWTs (15 min) + refresh tokens stored in DB
- No external auth provider
- Cost: $0 (it's just code)

---

### API Layer — Single Go Binary on Cloud Run

```go
// The whole API is one binary
cmd/bird/main.go
  ├── routes
  │   ├── web (HTML + HTMX: /, /timeline, /global, /u/{username}, /login, /register)
  │   ├── htmx partials (/htmx/timeline, /htmx/global, etc.)
  │   ├── api/v1 (JSON: /api/v1/tweets, /api/v1/users, etc.)
  │   └── admin (/api/v1/admin/* — agent endpoints)
  ├── middleware (auth, rate-limit, csrf, sync-moderation)
  ├── fanout (async outbox processor goroutine)
  ├── repository (PostgresRepo via pgx/v5)
  └── tmpl (Go html/template + HTMX content negotiation)
```

Why Go:
- **~100-300ms cold starts** (critical for scale-to-zero)
- Single static binary → tiny container image (~12-15MB with `scratch` base)
- Excellent concurrency for async fan-out
- Low memory footprint (Cloud Run bills for memory too)

Frontend: **Go `html/template` + HTMX**
- Server-rendered HTML, no JS build step
- HTMX for interactivity (infinite scroll, follow/unfollow, tweet posting)
- User profiles at `/u/{username}` to avoid route conflicts
- CSRF tokens via meta tag + HTMX `hx-headers` attribute
- `-dev` flag for template hot-reload during development

---

### CDN / Edge — Cloudflare (Free Tier)

- DNS, DDoS protection, edge caching
- Cache GET /timeline responses with 10-30s TTL
- **Rate limiting rules** (free tier allows 1 rule) — first line of defense before requests even hit Cloud Run
- Free tier covers enormous traffic
- Cost: $0

---

### Synchronous Moderation — The Write Path Safety Gate

This is the biggest architectural change from v0.2. **No tweet goes live without passing moderation.**

```
POST /tweet
  → Auth check
  → Rate limit check (Upstash Redis)
  → Content moderation (synchronous, in-process)
  → Write to DB + outbox
  → Return 200
```

#### Moderation Pipeline (fast path first, slow path only if needed)

```
1. Regex / blocklist check              (~0ms,  catches obvious slurs, spam URLs)
   ↓ pass
2. Lightweight classifier (local model)  (~5ms,  catches most spam/abuse patterns)
   ↓ uncertain
3. Claude Haiku API call                 (~200ms, nuanced content classification)
   ↓ uncertain
4. Hold for async review by Moderator agent (tweet marked "pending", not visible)
```

**Key insight:** Most tweets are fine. Step 1 catches the obvious bad stuff with zero cost. Step 2 catches most of the rest with a tiny local model (something like a fine-tuned DistilBERT running in the Go process via ONNX). Only genuinely ambiguous content hits the LLM API. Maybe 1-2% of tweets.

**Cost impact:** At 1M users / 10M tweets per day:
- ~95% pass at steps 1-2: $0
- ~4% hit Haiku: 400K calls/day × ~$0.0001/call = ~$40/day = ~$1,200/mo
- ~1% held for async review: handled by Moderator agent in batch

This is more expensive than v0.2 claimed but it's honest and it means zero harmful content goes live.

---

### Agent Architecture — Fully Autonomous with Safety Nets

#### Agent Runtime

Each agent is a **Cloud Run Job** triggered by **Cloud Scheduler** (cron) or by the API via **Pub/Sub** (events):
- Scale to zero between runs
- Scoped IAM permissions (principle of least privilege)
- Every action logged to an `agent_actions` table with full reasoning

#### Agent Roster

| Agent | Trigger | LLM Tier | What It Does |
|-------|---------|----------|--------------|
| **Moderator** | Pub/Sub (held tweets) + cron (every 10 min for appeals) | Sonnet | Reviews held tweets, processes user reports, handles appeals |
| **Anti-Spam** | Cron (every 5 min) | Haiku | Behavioral analysis: signup velocity, posting patterns, link spam rings. Shadowban/suspend. |
| **Ops/SRE** | Cloud Monitoring alert → Pub/Sub | Haiku + Sonnet for incidents | Reads GCP metrics, adjusts max-instances, triggers rollbacks, posts to alert webhook |
| **Support** | Cron (every 15 min, checks support queue) | Sonnet | Replies to user tickets, resolves account issues, escalates policy questions to Moderator |
| **Curator** | Cron (every 15 min) | Haiku | Computes trending topics, detects coordinated manipulation, flags to Anti-Spam |
| **Auditor** | Cron (daily at 03:00 UTC) | Opus | Reviews all agent actions from past 24h, flags anomalies, reverses bad decisions, publishes transparency report |

#### Escalation Chain

```
Anti-Spam ──→ Moderator ──→ Auditor (reviews after the fact)
Support   ──→ Moderator (for policy calls)
Ops/SRE   ──→ self-healing (no escalation for most issues)
All agents ──→ Auditor (daily review of every action)
```

#### Safety Nets (What v0.2 Was Missing)

**Dead Man's Switch for the Auditor:**
- Cloud Scheduler triggers Auditor daily
- Auditor must POST to a healthcheck URL (e.g., Healthchecks.io free tier, or a simple Cloud Function) when it completes
- If the healthcheck misses **2 consecutive pings** (48 hours), an alert fires to a fallback webhook (email, Slack, PagerDuty free tier)
- This is the one place a human might get paged — but only if the entire autonomous system is broken

**Agent Action Limits:**
- Each agent has per-run action caps (e.g., Moderator can't ban more than 50 users in a single run)
- If a cap is hit, the agent stops and the Auditor is notified immediately
- Prevents a confused agent from mass-banning or mass-deleting

**Immutable Audit Log:**
- `agent_actions` table: agent_id, action_type, target_id, reasoning, timestamp, reversible (bool)
- Soft-delete only — agents mark content as hidden, never hard-delete
- The Auditor can reverse any action by flipping visibility back
- Public transparency endpoint: `GET /transparency` returns aggregate stats

**Who Audits the Auditor?**
- The Auditor's own actions are logged to the same `agent_actions` table
- A simple Cloud Function (or cron script) runs weekly and checks:
  - Did the Auditor run every day?
  - Did it reverse an unusual number of actions? (anomaly detection)
  - Did its transparency report get published?
- If any check fails → alert to fallback webhook
- This is deterministic code, not an LLM — it can't have "opinions" or drift

---

### Observability — GCP Free Tier

| Component | What | Cost |
|-----------|------|------|
| **Cloud Logging** | All stdout/stderr from Cloud Run services and jobs | Free up to 50GB/mo |
| **Cloud Monitoring** | CPU, memory, request count, latency metrics | Free for GCP services |
| **Cloud Trace** | Request latency tracing | Free tier: 2.5M spans/mo |
| **Uptime Checks** | Ping the API, alert if down | Free: 1 per service |
| **Custom Metrics** | Tweets/sec, moderation latency, agent action counts | Free up to 150MB/mo |

The Ops agent queries these via the **GCP Monitoring API** (free). No Grafana, no Datadog — use what's already there.

---

### Infrastructure Safety — Cost Circuit Breakers

| Protection | Mechanism | Cost |
|------------|-----------|------|
| **Max Cloud Run instances** | `--max-instances=10` (increase per phase) | $0 |
| **GCP Budget Alert** | Alert at $50, $100, $200 thresholds | $0 |
| **Cloudflare rate limiting** | 1 free rate limiting rule (e.g., 100 req/min per IP) | $0 |
| **Account creation throttle** | Max 5 accounts per IP per hour (checked via Upstash Redis) | ~$0 |
| **Cloud Run request timeout** | 60s max — prevents long-running abuse | $0 |
| **Pub/Sub dead letter queue** | Failed agent triggers don't retry infinitely | ~$0 |

---

### Cost Model (Realistic)

| Component | 0 users | 1K users | 100K users | 1M users |
|-----------|---------|----------|------------|----------|
| Cloud Run (API) | $0 | ~$3 | ~$40 | ~$200 |
| Neon Postgres | $0 | $0 (free tier) | ~$30 | ~$120 |
| Upstash Redis | $0 | ~$1 | ~$15 | ~$60 |
| Upstash Kafka | $0 | $0 | ~$15 | ~$60 |
| Cloud Storage | $0 | ~$0.10 | ~$5 | ~$30 |
| Cloudflare | $0 | $0 | $0 | $0 |
| GCP Observability | $0 | $0 | $0 | ~$10 |
| Sync moderation (Haiku) | $0 | ~$3 | ~$150 | ~$1,200 |
| Agent LLM costs (all agents) | $0 | ~$5 | ~$50 | ~$300 |
| Meilisearch (Phase 3+) | $0 | $0 | ~$7 | ~$30 |
| **Total** | **$0** | **~$12** | **~$312** | **~$2,010** |

> **v0.2 claimed $740 at 1M users. v0.3 said ~$2,010. v0.4 is the same cost but simpler** — no SQLite→Postgres migration cost, no Litestream, no max-instances=1 constraint. Same money, better architecture.

---

### Deployment Strategy

```
GitHub repo
  ├── cmd/bird/main.go         → entry point, wiring, graceful shutdown
  ├── internal/
  │   ├── config/              → env-based config
  │   ├── model/               → domain types
  │   ├── repository/          → Repository interface + PostgresRepo
  │   ├── auth/                → JWT + argon2id + middleware
  │   ├── ratelimit/           → Upstash Redis sliding window + middleware
  │   ├── moderation/          → regex blocklist (LLM later)
  │   ├── fanout/              → outbox processor goroutine
  │   ├── handler/             → web + API + admin handlers
  │   ├── router/              → route registration + middleware stacking
  │   └── tmpl/                → template renderer + content negotiation
  ├── web/
  │   ├── templates/           → Go html/template + HTMX partials
  │   └── static/              → CSS, vendored htmx.min.js
  ├── agents/                  → Agent definitions + tool schemas (Phase 2)
  ├── migrations/              → Postgres SQL migrations
  ├── deploy/
  │   └── Dockerfile           → multi-stage, scratch base
  ├── Makefile
  └── docker-compose.yml       → Local dev (Postgres via Docker)
```

CI/CD:
- GitHub Actions: build → test → deploy to Cloud Run
- Migrations run as a Cloud Run Job before deploy
- Ops agent monitors deploy health, can trigger rollback via `gcloud` CLI
- All deploys tagged in git, rollback = redeploy previous tag

---

### Migration Path (Scale Phases)

```
Phase 0: $0/mo     — Zero users. Everything sleeps. Neon Postgres free tier.
Phase 1: ~$12/mo   — Hundreds of users. Postgres does everything. Upstash for rate limiting.
Phase 2: ~$100/mo  — Thousands. Add timeline caching in Redis. Add Upstash Kafka.
Phase 3: ~$300/mo  — Tens of thousands. Neon Pro. Meilisearch on GCE. Sync moderation costs ramp.
Phase 4: ~$1K/mo   — Hundreds of thousands. Neon read replicas. Optimize moderation (better local model = fewer Haiku calls).
Phase 5: ~$2K+/mo  — Millions. Dedicated infra for hot paths. Consider open-weight models for moderation to cut LLM costs.
```

Each phase is a natural evolution, not a rewrite. No database migration at any phase — Postgres scales with you.

---

### Trade-offs and Honest Limitations

| Decision | Upside | Downside |
|----------|--------|----------|
| Neon Postgres from day one | $0 idle, multi-instance, no migration, tsvector search built-in | Managed service dependency |
| Go + HTMX (not SPA) | No build step, fast cold starts, server-rendered, simple | Less interactive than React/Vue, requires full page concepts |
| User profiles at `/u/{username}` | No route conflicts, clean URL namespace | Slightly less clean than `/{username}` |
| CSRF tokens for web forms | Defense in depth for cookie-based auth | Small implementation overhead |
| Upstash Redis from day one | $0 idle, needed for rate limiting | Managed service, vendor dependency |
| Sync moderation in write path | No harmful content goes live | Adds ~200ms latency for ~5% of tweets (Haiku calls) |
| Local regex/classifier before LLM | Fast, free, reduces LLM calls by ~95% | Must maintain blocklist, risk of false positives |
| No Meilisearch until Phase 3 | Fewer services, less complexity | Postgres FTS is less feature-rich |
| Agent action caps | Prevents runaway agent errors | Legitimate mass-actions get throttled |
| Dead man's switch for Auditor | Catches total system failure | One human must monitor the webhook (catastrophic failure only) |
| Soft-delete only | Everything is reversible | Storage grows forever (mitigate with archival later) |

---

## Open Questions for Future Versions

- [x] **Frontend**: ~~Go templates + HTMX vs SPA~~ → Decided: Go templates + HTMX
- [ ] **DMs**: Encrypted messaging or keep it public-only like early Twitter?
- [ ] **Federation**: ActivityPub support to interop with Mastodon/Bluesky?
- [ ] **Open-weight moderation**: Fine-tune Llama/Mistral for content classification to eliminate Haiku costs at scale?
- [ ] **Multi-region**: Cloud Run multi-region with Neon read replicas?
- [ ] **Revenue model**: If this costs ~$0-2K/mo to run, what's the business model?
- [ ] **Media**: Photo/video uploads? Changes storage and moderation dramatically.
- [ ] **The 1% problem**: Tweets held for async review — what's the UX? "Your tweet is being reviewed" or silent delay?

---
---

## Previous Versions

### v0.3 — Hardened, Zero-Cost, Autonomous (SQLite + Litestream)

Hardened v0.2 with honest cost model, sync moderation, async fan-out, infrastructure safety nets.

#### Key Ideas (Retained in v0.4)
- Sync moderation in write path, async fan-out via outbox, agent safety nets (Auditor, dead man's switch, action caps), Upstash Redis from day one, GCP observability, infrastructure circuit breakers.

#### Why Superseded
- SQLite + Litestream forces `max-instances=1` — not scalable
- Litestream adds operational complexity (restore on cold start, async replication window)
- SQLite → Postgres migration at ~1K users is a significant effort, not a config toggle
- FTS5 → tsvector migration adds more work
- No CSRF protection, no frontend decision, fragile route namespacing

*Superseded by v0.4.*

---

### v0.1 — Managed Services + One Part-Time Human

The starting point: use best-in-class managed services, accept some fixed costs, keep one human in the loop for edge cases.

#### Write Path
- **Tweet Service**: Stateless containers (Cloud Run)
- **Storage**: PlanetScale (Vitess) or CockroachDB
- **Event Bus**: Redpanda or Confluent Kafka

#### Read Path (Timelines)
- Fan-out-on-write for normal users (< 10K followers)
- Fan-out-on-read for high-follower accounts
- Redis Cluster for cache

#### Agent Roles
Moderation, Trust & Safety, Ops, Support, Trending — all with human escalation.

#### Estimated Cost: ~$800–1,100/mo at 1M users

#### Why Superseded
Fixed minimum costs at zero users. Still required a human. Vendor lock-in.

*Superseded by v0.2.*

---

### v0.2 — Fully Autonomous, Zero-Cost-at-Zero-Users, "All Open Source"

Moved to fully agent-operated, serverless everything, scale-to-zero.

#### Key Ideas (Retained in v0.3)
- Cloud Run for all compute
- Serverless pricing everywhere (pay-per-request)
- Agent roster: Moderator, Anti-Spam, Ops, Support, Curator, Auditor
- Auditor as autonomous oversight

#### Why Superseded
- "100% open source" claim was dishonest (Neon, Upstash are SaaS)
- LLM cost model was 3-5x too low
- Cold starts unaddressed
- Meilisearch on Cloud Run doesn't work (volumes lost on scale-to-zero)
- Fan-out was synchronous in request path (time bomb)
- In-memory rate limiting doesn't survive scale-to-zero
- Agent event triggers were vague
- No safety nets: no one audits the Auditor, no infra abuse protection, no observability

*Superseded by v0.3.*
