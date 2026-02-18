# Plan/Apply Workflow

Inspired by Terraform:

1. **`bear plan`** — Detects changes, validates in parallel, writes `.bear/plan.yml`
2. **`bear apply`** — Reads the plan, deploys in parallel, updates lock file

The plan file is a checkpoint. You can review it, pass it through approval gates, or run it later. It's removed after `bear apply`.
