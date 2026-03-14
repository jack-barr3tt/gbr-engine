# gbr-engine

Work-in-progress project to consume Network Rail's Open Data Feeds and reason about train movements.

## Dev setup

- **Prereqs:** Go, tmux, [air](https://github.com/air-verse/air), [Atlas](https://atlasgo.io). Postgres, Redis, RabbitMQ running locally.
- Copy `.env.template` → `.env`, fill in secrets (NR feeds, reference API key, Postgres password, etc.).
- **Run:** `./dev.sh` — applies schema via Atlas, starts tmux session `gbr-dev` with 6 windows (api, queuer, trust, vstp, fetcher, schedule). Attach and work in each.
- **Schema:** `atlas schema apply --env local` (from repo root). `atlas schema diff --env local` to preview.
- **Stop:** `tmux kill-session -t gbr-dev`
