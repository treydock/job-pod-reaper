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
	"os"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	lifetimeAnnotation string = "pod.kubernetes.io/lifetime"
)

var (
	runOnce = kingpin.Flag("run-once",
		"Whether to run in loop (true) or run once like via cron (false)").Default("false").Envar("RUN_ONCE").Bool()
	reapMax = kingpin.Flag("reap-max",
		"Maximum Pods to reap in each run, set to 0 to disable this limit").Default("30").Envar("REAP_MAX").Int()
	reapInterval    = kingpin.Flag("reap-interval", "Duration between repear runs").Default("60s").Envar("REAP_INTERLVAL").Duration()
	reapNamespaces  = kingpin.Flag("reap-namespaces", "Namespaces to reap").Default("all").Envar("REAP_NAMESPACES").String()
	namespaceLabels = kingpin.Flag("namespace-labels", "Labels to use when filtering namespaces").Default("").Envar("NAMESPACE_LABELS").String()
	podsLabels      = kingpin.Flag("pods-labels", "Labels to use when filtering pods").Default("").Envar("PODS_LABELS").String()
	jobLabel        = kingpin.Flag("job-label", "Label to associate pod job with other objects").Default("job").Envar("JOB_LABEL").String()
	kubeconfig      = kingpin.Flag("kubeconfig", "Path to kubeconfig when running outside Kubernetes cluster").Default("").Envar("KUBECONFIG").String()
	logLevel        = kingpin.Flag("log-level", "Log level, One of: [debug, info, warn, error]").Default("info").Envar("LOG_LEVEL").String()
	logFormat       = kingpin.Flag("log-format", "Log format, One of: [logfmt, json]").Default("logfmt").Envar("LOG_FORMAT").String()
	timestampFormat = log.TimestampFormat(
		func() time.Time { return time.Now().UTC() },
		"2006-01-02T15:04:05.000Z07:00",
	)
	timeNow = time.Now
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

func main() {
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

	for {
		run(clientset, logger)
		if *runOnce {
			break
		} else {
			level.Debug(logger).Log("msg", "Sleeping...", "interval", fmt.Sprintf("%.0f", (*reapInterval).Seconds()))
			time.Sleep(*reapInterval)
		}
	}
}
func run(clientset kubernetes.Interface, logger log.Logger) {
	namespaces, err := getNamespaces(clientset, logger)
	if err != nil {
		level.Error(logger).Log("msg", "Error getting namespaces", "err", err)
		return
	}
	jobs, err := getJobs(clientset, namespaces, logger)
	if err != nil {
		level.Error(logger).Log("msg", "Error getting jods", "err", err)
		return
	}
	jobObjects, err := getJobObjects(clientset, jobs, logger)
	if err != nil {
		level.Error(logger).Log("msg", "Error getting job objects", "err", err)
		return
	}
	reap(clientset, jobObjects, logger)
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
		jobObjects = append(jobObjects, jobObject{objectType: "pod", name: job.podName, namespace: job.namespace})
		jobLogger := log.With(logger, "job", job.jobID, "namespace", job.namespace)
		listOptions := metav1.ListOptions{
			LabelSelector: fmt.Sprintf("%s=%s", *jobLabel, job.jobID),
		}
		services, err := clientset.CoreV1().Services(job.namespace).List(context.TODO(), listOptions)
		if err != nil {
			level.Error(jobLogger).Log("msg", "Error getting services", "err", err)
			return nil, err
		}
		for _, service := range services.Items {
			jobObject := jobObject{objectType: "service", name: service.Name, namespace: service.Namespace}
			jobObjects = append(jobObjects, jobObject)
		}
		configmaps, err := clientset.CoreV1().ConfigMaps(job.namespace).List(context.TODO(), listOptions)
		if err != nil {
			level.Error(jobLogger).Log("msg", "Error getting config maps", "err", err)
			return nil, err
		}
		for _, configmap := range configmaps.Items {
			jobObject := jobObject{objectType: "configmap", name: configmap.Name, namespace: configmap.Namespace}
			jobObjects = append(jobObjects, jobObject)
		}
		secrets, err := clientset.CoreV1().Secrets(job.namespace).List(context.TODO(), listOptions)
		if err != nil {
			level.Error(jobLogger).Log("msg", "Error getting secrets", "err", err)
			return nil, err
		}
		for _, secret := range secrets.Items {
			jobObject := jobObject{objectType: "secret", name: secret.Name, namespace: secret.Namespace}
			jobObjects = append(jobObjects, jobObject)
		}
	}
	return jobObjects, nil
}

func reap(clientset kubernetes.Interface, jobObjects []jobObject, logger log.Logger) {
	deletedPods := 0
	deletedServices := 0
	deletedConfigMaps := 0
	deletedSecrets := 0
	for _, job := range jobObjects {
		reapLogger := log.With(logger, "job", job.jobID, "name", job.name, "namespace", job.namespace)
		switch job.objectType {
		case "pod":
			err := clientset.CoreV1().Pods(job.namespace).Delete(context.TODO(), job.name, metav1.DeleteOptions{})
			if err != nil {
				level.Error(reapLogger).Log("msg", "Error deleting pod", "err", err)
				continue
			}
			level.Info(reapLogger).Log("msg", "Pod deleted")
			deletedPods++
		case "service":
			err := clientset.CoreV1().Services(job.namespace).Delete(context.TODO(), job.name, metav1.DeleteOptions{})
			if err != nil {
				level.Error(reapLogger).Log("msg", "Error deleting service", "err", err)
				continue
			}
			level.Info(reapLogger).Log("msg", "Service deleted")
			deletedServices++
		case "configmap":
			err := clientset.CoreV1().ConfigMaps(job.namespace).Delete(context.TODO(), job.name, metav1.DeleteOptions{})
			if err != nil {
				level.Error(reapLogger).Log("msg", "Error deleting config map", "err", err)
				continue
			}
			level.Info(reapLogger).Log("msg", "ConfigMap deleted")
			deletedConfigMaps++
		case "secret":
			err := clientset.CoreV1().Secrets(job.namespace).Delete(context.TODO(), job.name, metav1.DeleteOptions{})
			if err != nil {
				level.Error(reapLogger).Log("msg", "Error deleting secret", "err", err)
				continue
			}
			level.Info(reapLogger).Log("msg", "Secret deleted")
			deletedSecrets++
		}
	}
	level.Info(logger).Log("msg", "Reap summary",
		"pods", deletedPods, "services", deletedServices, "configmaps", deletedConfigMaps, "secrets", deletedSecrets)
}
