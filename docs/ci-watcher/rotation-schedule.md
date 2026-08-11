# CI Watcher Rotation Schedule

## Schedule

The rotation is weekly (Monday 09:00 UTC to Monday 09:00 UTC), managed via a YAML schedule in app-interface following the same pattern as `ocm-rosa-ic`.

- Schedule file: `data/teams/sd-sre/schedules/rosa-ci-watcher.yml` in app-interface
- Generated quarterly by a Claude Code cron, submitted as a GitLab MR
- No PagerDuty schedule is needed

## Rotation Structure

Each week has **3 ICs**, one drawn from each Service Engineering sub-pillar:

| Sub-Pillar | Pool |
|------------|------|
| Trust Engineering | Trust Engineering ICs |
| Production Engineering | Production Engineering ICs |
| Service Engineering | Service Engineering ICs |

The schedule is generated quarterly using FIFO priority rules (same logic as `ocm-rosa-ic`): people who haven't appeared recently go first, geographic diversity is considered, and new members are paired with experienced ones.

## Slack

- **`@rosa-ci-watcher`**: Slack alias pointing to the current 3 ICs, auto-synced from the app-interface schedule. Anyone can `@rosa-ci-watcher` in Slack to reach the current shift
- **`@rosa-ci-team`**: Slack handle that includes all rotation members

## When You Are Not Available

### Absent for 1 or 2 Days

- It is ok to skip the day(s) when you are not available
- Make sure triage states on the CI Health dashboard are current before you leave
- Review the results when you are back if you are away at the beginning or middle of your shift
- If there are any AI Agents running, do not let them run in the background when you are not around

### Absent for More Than 2 Days

- You **must** swap your shift with someone else **in your sub-pillar**
- Ping `@rosa-ci-team` in [#wg-rosa-cicd](https://redhat-internal.slack.com/archives/C0ADGRNAT8U) to find your replacement
- Submit an app-interface MR to update the schedule YAML with the swap
