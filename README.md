[![CI Status](https://github.com/OSC/job-pod-reaper/workflows/test/badge.svg?branch=main)](https://github.com/OSC/job-pod-reaper/actions?query=workflow%3Atest)
[![GitHub release](https://img.shields.io/github/v/release/OSC/job-pod-reaper?include_prereleases&sort=semver)](https://github.com/OSC/job-pod-reaper/releases/latest)
![GitHub All Releases](https://img.shields.io/github/downloads/OSC/job-pod-reaper/total)
![Docker Pulls](https://img.shields.io/docker/pulls/ohiosupercomputer/job-pod-reaper)
[![Go Report Card](https://goreportcard.com/badge/github.com/OSC/job-pod-reaper?ts=1)](https://goreportcard.com/report/github.com/OSC/job-pod-reaper)
[![codecov](https://codecov.io/gh/OSC/job-pod-reaper/branch/main/graph/badge.svg)](https://codecov.io/gh/OSC/job-pod-reaper)

# job-pod-reaper

Kubernetes service that can reap pods that have run past their lifetime.

This reaping is intended to be run against pods that act like short lived jobs.  Additional resources with the same `job` label as the expired pod will also be reaped.

Current list of resources that can be reaped:

* Pod
* Service
* ConfigMap
* Secret

Metrics about the count of reaped resources, duration of last reaping, and error counts can be queried using Prometheus `/metrics` endpoint exposed as a Service on port `8080`.

## Kubernetes support

Currently this code is built and tested against Kubernetes 1.19.

## Install

### Install with Helm

Only Helm >= 3.2.0 is supported.

```
helm repo add job-pod-reaper https://osc.github.io/job-pod-reaper
helm install job-pod-reaper job-pod-reaper/job-pod-reaper -n job-pod-reaper --create-namespace
```

For Open OnDemand the following adjustments can be made to get a working install using Helm:

```
helm install job-pod-reaper job-pod-reaper/job-pod-reaper \
-n job-pod-reaper --create-namespace \
--set config.reapNamespaces=false \
--set config.namespaceLabels='app.kubernetes.io/name=open-ondemand' \
--set config.podsLabels='app.kubernetes.io/managed-by=open-ondemand'
```

See [Cluster Role Bindings](#cluster-role-bindings) for information on necessary RoleBinding needed to allow job-pod-reaper to reap OnDemand pods if not reaping all namespaces.

### Install with YAML

First install the necessary Namespace and RBAC resources:

```
kubectl apply -f https://github.com/OSC/job-pod-reaper/releases/latest/download/namespace-rbac.yaml
```

For Open OnDemand a deployment can be installed using Open OnDemand specific deployment:

```
kubectl apply -f https://github.com/OSC/job-pod-reaper/releases/latest/download/ondemand-deployment.yaml
```

A more generic deployment:

```
kubectl apply -f https://github.com/OSC/job-pod-reaper/releases/latest/download/deployment.yaml
```

### Cluster Role Bindings

If you wish to authorize the job-pod-reaper to reap only specific namespaces, those namespaces will need to have the following RoleBinding added (replace `$NAMESPACE` with namespace name). Use this `RoleBinding` on namespaces listed with `--reap-namespaces` or if those namespaces match labels defined with `--namespace-labels`

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: $NAMESPACE-job-pod-reaper-rolebinding
  namespace: $NAMESPACE
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: job-pod-reaper
subjects:
- kind: ServiceAccount
  name: job-pod-reaper
  namespace: job-pod-reaper
```

If you wish to authorize job-pod-reader for all namespaces the following `ClusterRoleBinding` is required.  This would be needed if `--namespace-labels` is not defined and you set `--reap-namespaces=all`.

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: job-pod-reaper
  namespace: job-pod-reaper
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: job-pod-reaper
subjects:
- kind: ServiceAccount
  name: job-pod-reaper
  namespace: job-pod-reaper
```

## Configuration

To give a lifetime to your pods, add the following annotation:

`pod.kubernetes.io/lifetime: $DURATION`

`DURATION` has to be a [valid golang duration string](https://golang.org/pkg/time/#ParseDuration).

Example: `pod.kubernetes.io/lifetime: 24h`

The above annotation will cause the pod to be reaped (killed) once it reaches the age of 1d (24h)

## Deployment Details

The job-pod-reaper is intended to be deployed inside a Kubernetes cluster. It can also be run outside the cluster via cron.

The following flags and environment variables can modify the behavior of the job-pod-reaper:

| Flag    | Environment Variable | Description |
|---------|----------------------|-------------|
| --run-once            | RUN_ONCE=true       | Set to only execute reap code once and exit, ie used when run via cron|
| --reap-max=30         | REAP_MAX=30         | The maximum number of jobs to reap during each loop                   |
| --reap-interval=60s   | REAP_INTERVAL=60s   | [Duration](https://golang.org/pkg/time/#ParseDuration) between each reaping execution when run in loop |
| --reap-namespaces=all | REAP_NAMESPACES=all | Comma separated list of namespaces to reap, ignored if use --namespace-labels |
| --namespace-labels    | NAMESPACE_LABELS    | The labels to use when filtering namespaces to search, overrides --reap-namespaces |
| --pods-labels         | PODS_LABELS         | Comma separated list of Pod labels to filter which pods to reap       |
| --job-label=job       | JOB_LABEL=job       | The label associated to objects that represent a job to reap          |
| --kubeconfig          | KUBECONFIG          | The path to Kubernetes config, required when run outside Kubernetes   |
| --listen-address      | LISTEN_ADDRESS=:8080| Address to listen for HTTP requests                                   |
| --no-process-metrics  | PROCESS_METRICS=false | Disable metrics about the running processes such as CPU, memory and Go stats |
| --log-level=info      | LOG_LEVEL=info      | The logging level One of: [debug, info, warn, error]                  |
| --log-format=logfmt   | LOG_FORMAT=logfmt   | The logging format, either logfmt or json                             |
