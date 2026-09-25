# Operational SOPs

Tribal knowledge about how the ROSA CI ecosystem actually operates day to day, beyond the CI Watcher role docs and the suite setup guides.

- [`known-issues.md`](known-issues.md) - current bugs/gotchas affecting rosa-e2e, SRE operator e2e Prow CI, and shared CI infrastructure
- [`sapm-and-promotion.md`](sapm-and-promotion.md) - how SAPM (SaaS Auto Promotions Manager) gates operator deploys, gangway-bridge, and where the retrigger/promotion tooling lives
- [`escalation-and-ownership.md`](escalation-and-ownership.md) - who owns what (alert rules, CI job categories, build-farm infra) and how to escalate outside the CI Watcher's own remit
- [`day-to-day-operations.md`](day-to-day-operations.md) - what the CI Watcher / SRE Operator E2E owner actually does most days, with a worked example

For the CI Watcher rotation itself (role, daily runbook, escalation SLAs, schedule), see [`../ci-watcher/`](../ci-watcher/) - that's the canonical doc set, not duplicated here. For running/debugging the suite itself, see [`../ci-setup.md`](../ci-setup.md), [`../local-testing.md`](../local-testing.md), [`../prow-ci-cluster-profile.md`](../prow-ci-cluster-profile.md), and [`../trigger-prow-ci-manually.md`](../trigger-prow-ci-manually.md).

Broader ROSA CI/CD architecture (release validation, promotion pipelines, SRE alert ownership reference tables) lives in a separate design-docs repo, `hcm-design/cicd/docs/` - referenced by description below since it's a different git repo and relative links won't resolve.
