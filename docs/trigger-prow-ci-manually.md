# Triggering CI Jobs Manually via Gangway API

This document describes how to manually trigger CI jobs using the Gangway CI API.

## Prerequisites

- Access to the OpenShift CI console via SSO
- The `oc` CLI installed locally

## Steps

### 1. Get the CLI Login Command

Open the following link in your browser and log in with SSO:

https://oauth-openshift.apps.ci.l2s4.p1.openshiftapps.com/oauth/token/display

After login, the page will display the `oc login` command with your token directly.

### 2. Log in via the `oc` CLI

Run the login command from the previous step:

```bash
oc login --token=<your-token> --server=https://api.ci.l2s4.p1.openshiftapps.com:6443
```

### 3. Trigger a Job via the Gangway API

Use `curl` to trigger a job by name:

```bash
curl -X POST \
  -H "Authorization: Bearer $(oc whoami -t)" \
  -d '{"job_name": "${JOB_NAME}", "job_execution_type": "1"}' \
  https://gangway-ci.apps.ci.l2s4.p1.openshiftapps.com/v1/executions
```

Replace the `JOB_NAME` value with the name of the job you want to trigger.

Example:
```
curl -X POST \
  -H "Authorization: Bearer $(oc whoami -t)" \
  -d '{"job_name": "periodic-ci-openshift-online-rosa-e2e-main-periodics-rosa-hcp-e2e-stable-4-21", "job_execution_type": "1"}' \
  https://gangway-ci.apps.ci.l2s4.p1.openshiftapps.com/v1/executions
```

### Available Job Names

The CI job names follow the pattern `periodic-ci-openshift-release-main-nightly-<version>-<job>`. To find the exact job name, check the job definitions in the [openshift/release](https://github.com/openshift/release) repository or the CI dashboard.

## References

- [Triggering Prow Jobs via REST](https://docs.ci.openshift.org/how-tos/triggering-prowjobs-via-rest/)
