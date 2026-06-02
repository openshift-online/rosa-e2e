# CI Watcher Rotation Schedule

Jira: [ROSAENG-1427](https://redhat.atlassian.net/browse/ROSAENG-1427)

## Schedule

The rotation is weekly (Monday 00:00 UTC to Sunday 23:59 UTC), managed via PagerDuty schedule **ROSA CI Watcher**.

This is a **tracking schedule only** — there is no escalation policy and no paging.

See the [PagerDuty schedule](https://redhat.pagerduty.com/schedules/PGLVMVG) for the current and upcoming rotation assignments.

## Rotation Members

| Name | Org | Timezone |
|------|-----|----------|
| Dustin Row | SRE | NASA |
| Bo Meng | SRE | APAC |
| Ravi Trivedi | SRE | APAC |
| Daniel Hall | SRE | APAC |
| Lucas Ponce | OCM | EMEA |
| Josh Branham | SRE | NASA |
| Tim Williams | OCM | NASA |
| Jeff Frazier | OCM | NASA |

The rotation is intentionally cross-org (SRE + OCM) and cross-timezone (NASA, EMEA, APAC) to build shared understanding of the full CI surface.

## Slack

- **`@rosa-ci-watcher`**: Slack alias pointing to the current watcher, auto-synced from the PagerDuty schedule via app-interface. Anyone can `@rosa-ci-watcher` in Slack to reach the current watcher
- **`@rosa-ci-team`**: Slack handler that includes all rotation members

## When You Are Not Available

### Absent for 1 or 2 Days

- It is ok to skip the day(s) when you are not available
- Make sure the handover notes are ready if you are not available at the end of your shift
- Review the results when you are back if you are away at the beginning or middle of your shift
- If there are any AI Agents running, do not let them run in the background when you are not around

### Absent for More Than 2 Days

- You **must** swap your shift with someone else in the rotation if you are not available for more than half the working days of the shift
- Ping `@rosa-ci-team` in `#wg-rosa-ci-enhancement` to find your replacement
- The shift is weekly and not follow-the-sun — people in any region can swap
- Make sure the PagerDuty schedule override is taken in place correctly
