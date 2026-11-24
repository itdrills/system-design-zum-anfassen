# System design hands-on – Companion Code

Companion code for the book **System design hands-on** (German edition: *System Design zum Anfassen*) – a URL shortener that grows from one instance into a distributed system: load balancing, caching, replicas, queues and failures, every decision measured. Part of Thomas Zachmann's IT Drills series.

Book page: https://itdrills.com/books/system-design · German edition: https://it-trainingslager.de/buecher/system-design

The code accompanies the book; explanations and exercises are in the book. It is the state after Day 16 – in the book you build it file by file. Identifiers, comments and log messages are German, exactly as in the book's outputs.

## Contents

- `kurz/` – the URL shortener in Go (pgx, valkey-go, nats.go): cache-aside, read replica with fallback, sharding, idempotency keys, circuit breaker, timeouts
- `zaehler/` – worker that counts clicks from the NATS JetStream stream `KLICKS` exactly once
- `k6/` – load tests: `klick.js` (redirects), `anlegen.js` (create links with an API key), `wettkampf.js` (Day 16)
- `haproxy/haproxy.cfg` – leastconn, health checks, per-key quota, load shedding, L7 retries
- `systemd/` – unit files; settings come from drop-ins described in the book

## Quick start

```bash
git clone https://github.com/itdrills/system-design-zum-anfassen.git
cd system-design-zum-anfassen/kurz && go build -o kurz .
```

The lab (three Ubuntu VMs with PostgreSQL, PgBouncer, Valkey, NATS and HAProxy) is set up on Day 0. All passwords in this repo are lab values – never use them in production.

## License

Code: MIT (see LICENSE). Book text, figures and cover are not part of this repo and are not licensed.
