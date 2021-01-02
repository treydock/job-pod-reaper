// Copyright 2020 Ohio Supercomputer Center
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/version"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	appName            = "job-pod-reaper"
	lifetimeAnnotation = "pod.kubernetes.io/lifetime"
	metricsPath        = "/metrics"
	metricsNamespace   = "job_pod_reaper"
)

var (
	runOnce         = kingpin.Flag("run-once", "Set application to run once then exit, ie executed with cron").Default("false").Envar("RUN_ONCE").Bool()
	reapMax         = kingpin.Flag("reap-max", "Maximum Pods to reap in each run, set to 0 to disable this limit").Default("30").Envar("REAP_MAX").Int()
	reapInterval    = kingpin.Flag("reap-interval", "Duration between repear runs").Default("60s").Envar("REAP_INTERLVAL").Duration()
	reapNamespaces  = kingpin.Flag("reap-namespaces", "Namespaces to reap, ignored if --namespace-labels is set").Default("all").Envar("REAP_NAMESPACES").String()
	namespaceLabels = kingpin.Flag("namespace-labels", "Labels to use when filtering namespaces, causes --namespace-labels to be ignored").Default("").Envar("NAMESPACE_LABELS").String()
	podsLabels      = kingpin.Flag("pods-labels", "Labels to use when filtering pods").Default("").Envar("PODS_LABELS").String()
	jobLabel        = kingpin.Flag("job-label", "Label to associate pod job with other objects").Default("job").Envar("JOB_LABEL").String()
	kubeconfig      = kingpin.Flag("kubeconfig", "Path to kubeconfig when running outside Kubernetes cluster").Default("").Envar("KUBECONFIG").String()
	listenAddress   = kingpin.Flag("listen-address", "Address to listen for HTTP requests").Default(":8080").Envar("LISTEN_ADDRESS").String()
	processMetrics  = kingpin.Flag("process-metrics", "Collect metrics about running process such as CPU and memory and Go stats").Default("true").Envar("PROCESS_METRICS").Bool()
	logLevel        = kingpin.Flag("log-level", "Log level, One of: [debug, info, warn, error]").Default("info").Envar("LOG_LEVEL").String()
	logFormat       = kingpin.Flag("log-format", "Log format, One of: [logfmt, json]").Default("logfmt").Envar("LOG_FORMAT").String()
	timestampFormat = log.TimestampFormat(
		func() time.Time { return time.Now().UTC() },
		"2006-01-02T15:04:05.000Z07:00",
	)
	timeNow         = time.Now
	metricBuildInfo = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Name:      "build_info",
		Help:      "Build information",
		ConstLabels: prometheus.Labels{
			"version":   version.Version,
			"revision":  version.Revision,
			"branch":    version.Branch,
			"builddate": version.BuildDate,
			"goversion": version.GoVersion,
		},
	})
	metricReapedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Name:      "reaped_total",
			Help:      "Total number of object types reaped",
		},
		[]string{"type"},
	)
	metricError = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Name:      "error",
		Help:      "Indicates an error was encountered",
	})
	metricErrorsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Name:      "errors_total",
		Help:      "Total number of errors",
	})
	metricDuration = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Name:      "run_duration_seconds",
		Help:      "Last runtime duration in seconds",
	})
)

type podJob struct {
	jobID     string
	podName   string
	namespace string
}

type jobObject struct {
	objectType string
	jobID      string
	name       string
	namespace  string
}

func init() {
	metricBuildInfo.Set(1)
	metricReapedTotal.WithLabelValues("pod")
	metricReapedTotal.WithLabelValues("service")
	metricReapedTotal.WithLabelValues("configmap")
	metricReapedTotal.WithLabelValues("secret")
}

func main() {
	kingpin.Version(version.Print(appName))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	var logger log.Logger
	if *logFormat == "json" {
		logger = log.NewJSONLogger(log.NewSyncWriter(os.Stderr))
	} else {
		logger = log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	}
	switch *logLevel {
	case "debug":
		logger = level.NewFilter(logger, level.AllowDebug())
	case "info":
		logger = level.NewFilter(logger, level.AllowInfo())
	case "warn":
		logger = level.NewFilter(logger, level.AllowWarn())
	case "error":
		logger = level.NewFilter(logger, level.AllowError())
	default:
		logger = level.NewFilter(logger, level.AllowError())
		level.Error(logger).Log("msg", "Unrecognized log level", "level", *logLevel)
		os.Exit(1)
	}
	logger = log.With(logger, "ts", timestampFormat, "caller", log.DefaultCaller)

	var config *rest.Config
	var err error

	if *kubeconfig == "" {
		level.Info(logger).Log("msg", "Loading in cluster kubeconfig", "kubeconfig", *kubeconfig)
		config, err = rest.InClusterConfig()
	} else {
		level.Info(logger).Log("msg", "Loading kubeconfig", "kubeconfig", *kubeconfig)
		config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
	}
	if err != nil {
		level.Error(logger).Log("msg", "Error loading kubeconfig", "err", err)
		os.Exit(1)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		level.Error(logger).Log("msg", "Unable to generate Clientset", "err", err)
		os.Exit(1)
	}

	level.Info(logger).Log("msg", fmt.Sprintf("Starting %s", appName), "version", version.Info())
	level.Info(logger).Log("msg", "Build context", "build_context", version.BuildContext())

	http.Handle(metricsPath, promhttp.HandlerFor(metricGathers(), promhttp.HandlerOpts{}))

	go func() {
		if err := http.ListenAndServe(*listenAddress, nil); err != nil {
			level.Error(logger).Log("msg", "Error starting HTTP server", "err", err)
			os.Exit(1)
		}
	}()

	for {
		var errNum int
		err = run(clientset, logger)
		if err != nil {
			errNum = 1
		} else {
			errNum = 0
		}
		metricError.Set(float64(errNum))
		if *runOnce {
			os.Exit(errNum)
		} else {
			level.Debug(logger).Log("msg", "Sleeping for interval", "interval", fmt.Sprintf("%.0f", (*reapInterval).Seconds()))
			time.Sleep(*reapInterval)
		}
	}
}

func run(clientset kubernetes.Interface, logger log.Logger) error {
	start := timeNow()
	defer metricDuration.Set(time.Since(start).Seconds())
	namespaces, err := getNamespaces(clientset, logger)
	if err != nil {
		level.Error(logger).Log("msg", "Error getting namespaces", "err", err)
		return err
	}
	jobs, err := getJobs(clientset, namespaces, logger)
	if err != nil {
		level.Error(logger).Log("msg", "Error getting jods", "err", err)
		return err
	}
	jobObjects, err := getJobObjects(clientset, jobs, logger)
	if err != nil {
		level.Error(logger).Log("msg", "Error getting job objects", "err", err)
		return err
	}
	errCount := reap(clientset, jobObjects, logger)
	if errCount > 0 {
		err := fmt.Errorf("%d errors encountered during reap", errCount)
		level.Error(logger).Log("msg", err)
		return err
	}
	return nil
}

func getNamespaces(clientset kubernetes.Interface, logger log.Logger) ([]string, error) {
	var namespaces []string
	namespaces = strings.Split(*reapNamespaces, ",")
	if len(namespaces) == 1 && strings.ToLower(namespaces[0]) == "all" {
		namespaces = []string{metav1.NamespaceAll}
	}
	if *namespaceLabels != "" {
		namespaces = nil
		nsLabels := strings.Split(*namespaceLabels, ",")
		for _, label := range nsLabels {
			nsListOptions := metav1.ListOptions{
				LabelSelector: label,
			}
			level.Debug(logger).Log("msg", "Getting namespaces with label", "label", label)
			ns, err := clientset.CoreV1().Namespaces().List(context.TODO(), nsListOptions)
			if err != nil {
				level.Error(logger).Log("msg", "Error getting namespace list", "label", label, "err", err)
				return nil, err
			}
			level.Debug(logger).Log("msg", "Namespaces returned", "count", len(ns.Items))
			for _, namespace := range ns.Items {
				namespaces = append(namespaces, namespace.Name)
			}
		}

	}
	return namespaces, nil
}

func getJobs(clientset kubernetes.Interface, namespaces []string, logger log.Logger) ([]podJob, error) {
	labels := strings.Split(*podsLabels, ",")
	jobs := []podJob{}
	toReap := 0
	for _, ns := range namespaces {
		for _, l := range labels {
			listOptions := metav1.ListOptions{
				LabelSelector: l,
			}
			pods, err := clientset.CoreV1().Pods(ns).List(context.TODO(), listOptions)
			if err != nil {
				level.Error(logger).Log("msg", "Error getting pod list", "label", l, "namespace", ns, "err", err)
				metricErrorsTotal.Inc()
				return nil, err
			}
			for _, pod := range pods.Items {
				if *reapMax != 0 && toReap >= *reapMax {
					level.Info(logger).Log("msg", "Max reap reached, skipping rest", "max", *reapMax)
					return jobs, nil
				}
				podLogger := log.With(logger, "pod", pod.Name, "namespace", pod.Namespace)
				var lifetime time.Duration
				if val, ok := pod.Annotations[lifetimeAnnotation]; !ok {
					level.Debug(podLogger).Log("msg", "Pod lacks reaper annotation, skipping", "annotation", lifetimeAnnotation)
					continue
				} else {
					level.Debug(podLogger).Log("msg", "Found pod with reaper annotation", "annotation", val)
					lifetime, err = time.ParseDuration(val)
					if err != nil {
						level.Error(podLogger).Log("msg", "Error parsing annotation, SKIPPING", "annotation", val, "err", err)
						metricErrorsTotal.Inc()
						continue
					}
				}
				var jobID string
				if val, ok := pod.Labels[*jobLabel]; ok {
					level.Debug(podLogger).Log("msg", "Pod has job label", "job", val)
					jobID = val
				} else {
					level.Debug(podLogger).Log("msg", "Pod does not have job label, skipping")
					continue
				}
				currentLifetime := timeNow().Sub(pod.CreationTimestamp.Time)
				level.Debug(podLogger).Log("msg", "Pod lifetime", "lifetime", currentLifetime.Seconds())
				if currentLifetime > lifetime {
					level.Debug(podLogger).Log("msg", "Pod is past its lifetime and will be killed.")
					job := podJob{jobID: jobID, podName: pod.Name, namespace: pod.Namespace}
					jobs = append(jobs, job)
				}
			}
		}
	}
	return jobs, nil
}

func getJobObjects(clientset kubernetes.Interface, jobs []podJob, logger log.Logger) ([]jobObject, error) {
	jobObjects := []jobObject{}
	for _, job := range jobs {
		jobObjects = append(jobObjects, jobObject{objectType: "pod", jobID: job.jobID, name: job.podName, namespace: job.namespace})
		jobLogger := log.With(logger, "job", job.jobID, "namespace", job.namespace)
		listOptions := metav1.ListOptions{
			LabelSelector: fmt.Sprintf("%s=%s", *jobLabel, job.jobID),
		}
		services, err := clientset.CoreV1().Services(job.namespace).List(context.TODO(), listOptions)
		if err != nil {
			level.Error(jobLogger).Log("msg", "Error getting services", "err", err)
			metricErrorsTotal.Inc()
			return nil, err
		}
		for _, service := range services.Items {
			jobObject := jobObject{objectType: "service", jobID: job.jobID, name: service.Name, namespace: service.Namespace}
			jobObjects = append(jobObjects, jobObject)
		}
		configmaps, err := clientset.CoreV1().ConfigMaps(job.namespace).List(context.TODO(), listOptions)
		if err != nil {
			level.Error(jobLogger).Log("msg", "Error getting config maps", "err", err)
			metricErrorsTotal.Inc()
			return nil, err
		}
		for _, configmap := range configmaps.Items {
			jobObject := jobObject{objectType: "configmap", jobID: job.jobID, name: configmap.Name, namespace: configmap.Namespace}
			jobObjects = append(jobObjects, jobObject)
		}
		secrets, err := clientset.CoreV1().Secrets(job.namespace).List(context.TODO(), listOptions)
		if err != nil {
			level.Error(jobLogger).Log("msg", "Error getting secrets", "err", err)
			metricErrorsTotal.Inc()
			return nil, err
		}
		for _, secret := range secrets.Items {
			jobObject := jobObject{objectType: "secret", jobID: job.jobID, name: secret.Name, namespace: secret.Namespace}
			jobObjects = append(jobObjects, jobObject)
		}
	}
	return jobObjects, nil
}

func reap(clientset kubernetes.Interface, jobObjects []jobObject, logger log.Logger) int {
	deletedPods := 0
	deletedServices := 0
	deletedConfigMaps := 0
	deletedSecrets := 0
	errCount := 0
	for _, job := range jobObjects {
		reapLogger := log.With(logger, "job", job.jobID, "name", job.name, "namespace", job.namespace)
		switch job.objectType {
		case "pod":
			err := clientset.CoreV1().Pods(job.namespace).Delete(context.TODO(), job.name, metav1.DeleteOptions{})
			if err != nil {
				errCount++
				level.Error(reapLogger).Log("msg", "Error deleting pod", "err", err)
				metricErrorsTotal.Inc()
				continue
			}
			level.Info(reapLogger).Log("msg", "Pod deleted")
			metricReapedTotal.With(prometheus.Labels{"type": "pod"}).Inc()
			deletedPods++
		case "service":
			err := clientset.CoreV1().Services(job.namespace).Delete(context.TODO(), job.name, metav1.DeleteOptions{})
			if err != nil {
				errCount++
				level.Error(reapLogger).Log("msg", "Error deleting service", "err", err)
				metricErrorsTotal.Inc()
				continue
			}
			level.Info(reapLogger).Log("msg", "Service deleted")
			metricReapedTotal.With(prometheus.Labels{"type": "service"}).Inc()
			deletedServices++
		case "configmap":
			err := clientset.CoreV1().ConfigMaps(job.namespace).Delete(context.TODO(), job.name, metav1.DeleteOptions{})
			if err != nil {
				errCount++
				level.Error(reapLogger).Log("msg", "Error deleting config map", "err", err)
				metricErrorsTotal.Inc()
				continue
			}
			level.Info(reapLogger).Log("msg", "ConfigMap deleted")
			metricReapedTotal.With(prometheus.Labels{"type": "configmap"}).Inc()
			deletedConfigMaps++
		case "secret":
			err := clientset.CoreV1().Secrets(job.namespace).Delete(context.TODO(), job.name, metav1.DeleteOptions{})
			if err != nil {
				errCount++
				level.Error(reapLogger).Log("msg", "Error deleting secret", "err", err)
				metricErrorsTotal.Inc()
				continue
			}
			level.Info(reapLogger).Log("msg", "Secret deleted")
			metricReapedTotal.With(prometheus.Labels{"type": "secret"}).Inc()
			deletedSecrets++
		}
	}
	level.Info(logger).Log("msg", "Reap summary",
		"pods", deletedPods,
		"services", deletedServices,
		"configmaps", deletedConfigMaps,
		"secrets", deletedSecrets,
	)
	return errCount
}

func metricGathers() prometheus.Gatherers {
	registry := prometheus.NewRegistry()
	registry.MustRegister(metricBuildInfo)
	registry.MustRegister(metricReapedTotal)
	registry.MustRegister(metricError)
	registry.MustRegister(metricErrorsTotal)
	registry.MustRegister(metricDuration)
	gatherers := prometheus.Gatherers{registry}
	if *processMetrics {
		gatherers = append(gatherers, prometheus.DefaultGatherer)
	}
	return gatherers
}
