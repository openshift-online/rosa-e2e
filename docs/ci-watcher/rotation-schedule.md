# CI Watcher Rotation Schedule

## Schedule

The rotation is weekly (Monday 09:00 UTC to Monday 09:00 UTC), managed via a YAML schedule in app-interface.

- Schedule file: `data/teams/sd-sre/schedules/rosa-ci-watcher.yml` in app-interface
- Pool: all ICs in the ROSA org, excluding PMs, HyperFleet, and GovCloud/FedRAMP members. Source of truth: `config/structures/hybrid_platforms/rosa/` in [hybrid-platforms/org](https://gitlab.cee.redhat.com/hybrid-platforms/org)
- **Maintained monthly** by the CI Watcher weekly handover task (first Monday of each month). The handover checks for org changes and opens a GitLab MR against app-interface if the pool has changed
- No PagerDuty schedule is needed

## Rotation Structure

Each week has **1 IC** from the eligible pool, assigned in round-robin order. The pool covers the full ROSA org (excluding HyperFleet, GovCloud/FedRAMP, and PM roles), so the full rotation cycle spans as many weeks as there are eligible ICs.

## Slack

- **`@rosa-ci-watcher`**: Slack alias pointing to the current IC, auto-synced from the app-interface schedule. Anyone can `@rosa-ci-watcher` in Slack to reach the current shift
- **`@rosa-ci-team`**: Slack handle that includes all rotation members

## When You Are Not Available

### Absent for 1 or 2 Days

- It is ok to skip the day(s) when you are not available
- Make sure triage states on the CI Health dashboard are current before you leave
- Review the results when you are back if you are away at the beginning or middle of your shift
- If there are any AI Agents running, do not let them run in the background when you are not around

### Absent for More Than 2 Days

- You **must** swap your shift with someone else
- Ping `@rosa-ci-team` in [#wg-rosa-cicd](https://redhat-internal.slack.com/archives/C0ADGRNAT8U) to find your replacement
- Submit an app-interface MR to update the schedule YAML with the swap
