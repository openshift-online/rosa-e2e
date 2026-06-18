# Debugging FVT Jobs with RHOBS CS Telemetry

Clusters Service emits structured lifecycle events for every ROSA HCP cluster operation (provision, upgrade, deprovision). These events are forwarded to RHOBS Loki and can be queried in real time to debug FVT failures without waiting for the job to finish.

## Quick Start

1. Open Grafana Explore: https://grafana.app-sre.devshift.net/explore
2. Select the datasource for your environment:
   - Staging: `rhobs-us-east-1-0-stage-hcp-logs`
   - Production: `rhobs-us-east-1-0-production-hcp-logs`
3. Enter a LogQL query (see examples below)
4. Click "Live" in the top-right to stream logs in real time

## LogQL Query Examples

### All CS telemetry events (staging)

Shows every cluster lifecycle event happening right now on staging:

```logql
{k8s_namespace_name="uhc-stage"} |= "[ROSA HCP -"
```

### Filter by cluster name

If you know the cluster name (from FVT output or OCM):

```logql
{k8s_namespace_name="uhc-stage"} |= "[ROSA HCP -" |= "my-cluster-name"
```

### Filter by cluster ID

If you have the OCM cluster ID:

```logql
{k8s_namespace_name="uhc-stage"} |= "[cid='2r0dajovl5m732e86n0desbv622udrai']"
```

### Filter by FVT cluster name prefix

FVT clusters follow naming patterns. Filter by prefix to see all clusters from a specific test:

```logql
{k8s_namespace_name="uhc-stage"} |= "[ROSA HCP -" |= "cs-ci-longname"
```

### Installation events only

```logql
{k8s_namespace_name="uhc-stage"} |= "[ROSA HCP - IT"
```

### Deprovision events only

```logql
{k8s_namespace_name="uhc-stage"} |= "[ROSA HCP - UT"
```

### Control plane upgrade events

```logql
{k8s_namespace_name="uhc-stage"} |= "[ROSA HCP - UCT"
```

### Node pool upgrade events

```logql
{k8s_namespace_name="uhc-stage"} |= "[ROSA HCP - UNT"
```

### Errors for a specific cluster

```logql
{k8s_namespace_name="uhc-stage"} |= "my-cluster-name" |~ "(?i)(error|fail|timeout)"
```

## Telemetry Event Reference

All events are tagged with `[cid='<cluster_id>']` and `[opid='<operation_id>']` for correlation.

### Installation (IT)

| Event | Description |
|-------|-------------|
| IT1 | CS receives installation request |
| IT2 | CS creates/updates ManifestWork resources |
| IT3 | Service Cluster applies ManifestWork to Management Cluster |
| IT4 | API Certificate is ready |
| IT5 | HostedCluster is available |
| IT6 | HostedCluster joins ACM hub |
| IT7 | CS marks cluster as ready |

### Deprovision (UT)

| Event | Description |
|-------|-------------|
| UT1 | CS receives deletion request |
| UT2.1/2.2 | Ingress cert and configmap deletion triggered |
| UT3.1/3.2 | Ingress cert and configmap confirmed deleted |
| UT4 | ManagedCluster deletion triggered |
| UT5 | ManagedCluster confirmed deleted |
| UT6.1/6.2 | HostedCluster and NodePool deletion triggered |
| UT7 | Non-namespace ManifestWorks confirmed deleted |
| UT8-UT9 | Namespace ManifestWork deletion |
| UT10-UT11 | Cluster namespace deletion |
| UT14-UT15 | Route53 HostedZone deletion |
| UT16-UT17 | AWS Instance Profile deletion |

### Control Plane Upgrade (UCT)

| Event | Description |
|-------|-------------|
| UCT1 | Upgrade policy created |
| UCT2 | Upgrade policy started |
| UCT3 | ManifestWork updated with new version |
| UCT4 | HyperShift propagates version to HCP |
| UCT5 | HyperShift propagates completion |
| UCT6 | CS updates version |

### Node Pool Upgrade (UNT)

| Event | Description |
|-------|-------------|
| UNT1 | Node pool upgrade policy created |
| UNT2 | Policy started |
| UNT3 | ManifestWork updated |
| UNT4 | ManifestWork applied on MC |
| UNT5 | NodePool available |
| UNT6 | CAPI MachineSets ready |
| UNT7 | CAPI Nodes ready |
| UNT8 | MC marks node pool with new version |
| UNT9 | CS updates node pool version |

### Node Pool Changes (NT)

| Event | Description |
|-------|-------------|
| NT1 | CS receives node pool change request |
| NT2 | ManifestWork created/updated |
| NT3 | ManifestWork applied on MC |
| NT4 | NodePool available |
| NT5 | CAPI MachineSets ready |
| NT6 | CAPI Nodes ready |

## Environments

| Environment | CS Namespace | Grafana Datasource |
|-------------|-------------|-------------------|
| Staging | `uhc-stage` | `rhobs-us-east-1-0-stage-hcp-logs` |
| Integration | `uhc-integration` | N/A (CS logs not forwarded yet) |
| Production | `uhc-production` | `rhobs-us-east-1-0-production-hcp-logs` |

## Prow Artifact Collection

FVT jobs with the `rosa-e2e-collect-cs-telemetry` post-step automatically dump a formatted telemetry report to Prow artifacts after each run. Look for `cs-telemetry.log` in the artifacts under the `rosa-e2e-collect-cs-telemetry` step.

## Further Reading

- CS telemetry docs: https://gitlab.cee.redhat.com/service/uhc-clusters-service/-/blob/master/docs/rosa_hcp/telemetry.md
- Jira: https://redhat.atlassian.net/browse/ROSAENG-60057
