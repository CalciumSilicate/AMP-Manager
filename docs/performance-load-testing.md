# Performance Load Testing

This repository includes a local performance lab for the AMP proxy hot path. The lab is designed to isolate AMP Manager itself by running against a controlled local mock upstream, while keeping PostgreSQL as the baseline database.

The lab intentionally avoids building Go code inside Docker during normal runs:

- `AMP Manager` runs from `ghcr.io/meglinge/amp-manager:latest` by default
- `mock upstream` runs as a host-side Go process started by `perf/run.sh`
- `Postgres` and `k6` still run in Docker

## What It Covers

- `Postgres + AMP Manager + mock upstream + k6`
- Seeded test data:
  - 1 admin user
  - configurable perf user count, default `5000`
  - 1 API key per user
  - 2 OpenAI channels:
    - chat completions
    - responses
  - per-user model mappings so requests pass through:
    - API key auth
    - billing check
    - model mapping
    - channel routing
    - request capture
    - proxying
    - request log and billing settlement

## Prerequisites

- Docker
- Go
- A locally available AMP Manager image, or access to pull `ghcr.io/meglinge/amp-manager:latest`

## Default Profiles

- `default`
  - request detail capture enabled
- `detail_off`
  - request detail capture disabled

## Quick Start

```bash
./perf/run.sh full
```

If you want to pin a different app image:

```bash
AMPMANAGER_IMAGE=ghcr.io/meglinge/amp-manager:latest ./perf/run.sh smoke
```

Common variants:

```bash
PROFILE=detail_off ./perf/run.sh full
./perf/run.sh smoke
./perf/run.sh nonstream
./perf/run.sh stream
./perf/run.sh mixed
```

Useful overrides:

```bash
USER_COUNT=5000 ACTIVE_KEY_POOL=500 ./perf/run.sh full
NONSTREAM_STEPS_RPM=600,1200,2000 ./perf/run.sh nonstream
STREAM_STEPS_CONCURRENCY=50,100,200 ./perf/run.sh stream
```

## Result Layout

Each run writes a timestamped directory under `perf/results/` with:

- `meta.json`
- `k6/*.json`
- `evaluations/*.json`
- `logs/*.log`
- `sql/*.txt`
- `REPORT.md`

## Notes

- The runner resets the perf stack by default with `docker compose down -v`.
- The stack is kept alive after the run by default for inspection. Set `KEEP_STACK=false` to stop it automatically.
- The mock upstream behavior is controlled through `perf/env.defaults` and optional overrides in `perf/.env.local`.
