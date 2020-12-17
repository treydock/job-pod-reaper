[![CI Status](https://github.com/OSC/job-pod-reaper/workflows/test/badge.svg?branch=main)](https://github.com/OSC/job-pod-reaper/actions?query=workflow%3Atest)
[![GitHub release](https://img.shields.io/github/v/release/OSC/job-pod-reaper?include_prereleases&sort=semver)](https://github.com/OSC/job-pod-reaper/releases/latest)
![GitHub All Releases](https://img.shields.io/github/downloads/OSC/job-pod-reaper/total)
![Docker Pulls](https://img.shields.io/docker/pulls/ohiosupercomputer/job-pod-reaper)
[![Go Report Card](https://goreportcard.com/badge/github.com/OSC/job-pod-reaper)](https://goreportcard.com/report/github.com/OSC/job-pod-reaper)
[![codecov](https://codecov.io/gh/OSC/job-pod-reaper/branch/master/graph/badge.svg)](https://codecov.io/gh/OSC/job-pod-reaper)

# job-pod-reaper

Kubernetes service that can reap pods that have run past their lifetime, or pods that have been evicted.

This reaping is intended to be run against pods that act like short lived jobs.  Additional resources with the same `job` label as the expired pod will also be reaped.

Current list of resources that can be reaped:

* Pod
* Service
* ConfigMap
* Secret

## Configuration

To give a lifetime to your pods, add the following annotation:

`pod.kubernetes.io/lifetime: $DURATION`

`DURATION` has to be a [valid golang duration string](https://golang.org/pkg/time/#ParseDuration).

A duration string is a possibly signed sequence of decimal numbers, each with optional fraction and a unit suffix, such as "300ms", "-1.5h" or "2h45m". Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".

Example: `pod.kubernetes.io/lifetime: 24h`

The above annotation will cause the pod to be reaped (killed) once it reaches the age of 1d (24h)

## Deployment Details

The job-pod-reaper is intended to be deployed inside a Kubernetes cluster. It can also be run outside the cluster via cron.

The following flags and environment variables can modify the behavior of the job-pod-reaper:

| Flag    | Environment Variable | Description |
|---------|----------------------|-------------|
| --run-once            | RUN_ONCE=true       | Set to only execute reap code once and exit, ie used when run via cron|
| --reap-max=30         | REAP_MAX=30         | The maximum number of jobs to reap during each loop                   |
| --reap-interval=60    | REAP_INTERVAL=60    | The number of seconds between each reaping execution when run in loop |
| --reap-namespaces=all | REAP_NAMESPACES=all | Comma separated list of namespaces to reap                            |
| --namespace-labels    | NAMESPACE_LABELS    | The labels to use when filtering namespaces to search, overrides --reap-namespaces |
| --pods-labels         | PODS_LABELS         | Comma separated list of Pod labels to filter which pods to reap       |
| --job-label=job       | JOB_LABEL=job       | The label associated to objects that represent a job to reap          |
| --kubeconfig          | KUBECONFIG          | The path to Kubernetes config, required when run outside Kubernetes   |
| --log.level=info      | LOG_LEVEL=info      | The logging level One of: [debug, info, warn, error]                  |
| --log.format=logfmt   | LOG_FORMAT=logfmt   | The logging format, either logfmt or json                             |