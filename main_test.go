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
	"os"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

var (
	noString     = ""
	podStart, _  = time.Parse("01/02/2006 15:04:05", "01/01/2020 13:00:00")
	podStartTime = metav1.NewTime(podStart)
	clientset    = fake.NewSimpleClientset(&v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "non-job",
		},
	}, &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "user-user1",
			Labels: map[string]string{
				"app.kubernetes.io/name": "open-ondemand",
			},
		},
	}, &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "user-user2",
			Labels: map[string]string{
				"app.kubernetes.io/name": "foo",
			},
		},
	}, &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "user-user3",
			Labels: map[string]string{
				"app.kubernetes.io/name": "open-ondemand-test",
			},
		},
	}, &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "non-job-pod",
			Namespace:   "non-job",
			Annotations: map[string]string{},
			Labels:      map[string]string{},
		},
	}, &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ondemand-job1",
			Namespace: "user-user1",
			Annotations: map[string]string{
				"pod.kubernetes.io/lifetime": "1h",
			},
			Labels: map[string]string{
				"job":                          "1",
				"app.kubernetes.io/managed-by": "open-ondemand",
			},
		},
		Status: v1.PodStatus{
			StartTime: &podStartTime,
		},
	}, &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ondemand-job2",
			Namespace: "user-user2",
			Annotations: map[string]string{
				"pod.kubernetes.io/lifetime": "30m",
			},
			Labels: map[string]string{
				"job":                          "2",
				"app.kubernetes.io/managed-by": "open-ondemand",
			},
		},
		Status: v1.PodStatus{
			StartTime: &podStartTime,
		},
	}, &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ondemand-job3",
			Namespace: "user-user3",
			Annotations: map[string]string{
				"pod.kubernetes.io/lifetime": "30m",
			},
			Labels: map[string]string{
				"job":                          "3",
				"app.kubernetes.io/managed-by": "open-ondemand",
			},
		},
		Status: v1.PodStatus{
			StartTime: nil,
		},
	}, &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-job1",
			Namespace: "user-user1",
			Labels: map[string]string{
				"job": "1",
			},
		},
	}, &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-job2",
			Namespace: "user-user2",
			Labels: map[string]string{
				"job": "2",
			},
		},
	}, &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "configmap-job1",
			Namespace: "user-user1",
			Labels: map[string]string{
				"job": "1",
			},
		},
	}, &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "configmap-job2",
			Namespace: "user-user2",
			Labels: map[string]string{
				"job": "2",
			},
		},
	}, &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret-job1",
			Namespace: "user-user1",
			Labels: map[string]string{
				"job": "1",
			},
		},
	}, &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret-job2",
			Namespace: "user-user2",
			Labels: map[string]string{
				"job": "2",
			},
		},
	})
)

func TestGetNamespaces(t *testing.T) {
	if _, err := kingpin.CommandLine.Parse([]string{}); err != nil {
		t.Fatal(err)
	}
	w := log.NewSyncWriter(os.Stderr)
	logger := log.NewLogfmtLogger(w)
	namespaces, err := getNamespaces(clientset, logger)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(namespaces) != 1 {
		t.Errorf("Unexpected number of namespaces: %d", len(namespaces))
	}
	if namespaces[0] != metav1.NamespaceAll {
		t.Errorf("Unexpected namespace, got: %v", namespaces[0])
	}
}

func TestGetNamespacesByLabel(t *testing.T) {
	if _, err := kingpin.CommandLine.Parse([]string{}); err != nil {
		t.Fatal(err)
	}
	labels := "app.kubernetes.io/name=open-ondemand"
	namespaceLabels = &labels
	w := log.NewSyncWriter(os.Stderr)
	logger := log.NewLogfmtLogger(w)
	namespaces, err := getNamespaces(clientset, logger)
	namespaceLabels = &noString
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(namespaces) != 1 {
		t.Errorf("Unexpected number of namespaces: %d", len(namespaces))
	}
	if namespaces[0] != "user-user1" {
		t.Errorf("Unexpected namespace, got: %v", namespaces[0])
	}
}

func TestGetJobs(t *testing.T) {
	if _, err := kingpin.CommandLine.Parse([]string{}); err != nil {
		t.Fatal(err)
	}
	w := log.NewSyncWriter(os.Stderr)
	logger := log.NewLogfmtLogger(w)

	labels := "app.kubernetes.io/managed-by=open-ondemand"
	podsLabels = &labels

	timeNow = func() time.Time {
		t, _ := time.Parse("01/02/2006 15:04:05", "01/01/2020 15:00:00")
		return t
	}

	namespaces, err := getNamespaces(clientset, logger)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	jobs, err := GetJobs(clientset, namespaces, logger)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("Expected 2 jobs, got %d", len(jobs))
		return
	}
	if val := jobs[0].jobID; val != "1" {
		t.Errorf("Unexpected jobID, got: %v", val)
	}
	if val := jobs[1].jobID; val != "2" {
		t.Errorf("Unexpected jobID, got: %v", val)
	}
}

func TestGetJobsCase1(t *testing.T) {
	if _, err := kingpin.CommandLine.Parse([]string{}); err != nil {
		t.Fatal(err)
	}
	w := log.NewSyncWriter(os.Stderr)
	logger := log.NewLogfmtLogger(w)

	labels := "app.kubernetes.io/managed-by=open-ondemand"
	podsLabels = &labels

	timeNow = func() time.Time {
		t, _ := time.Parse("01/02/2006 15:04:05", "01/01/2020 13:45:00")
		return t
	}

	namespaces, err := getNamespaces(clientset, logger)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	jobs, err := GetJobs(clientset, namespaces, logger)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(jobs) != 1 {
		t.Errorf("Expected 1 jobs, got %d", len(jobs))
		return
	}
	if val := jobs[0].jobID; val != "2" {
		t.Errorf("Unexpected jobID, got: %v", val)
	}
}

func TestGetJobsNamespaceLabels(t *testing.T) {
	if _, err := kingpin.CommandLine.Parse([]string{}); err != nil {
		t.Fatal(err)
	}
	w := log.NewSyncWriter(os.Stderr)
	logger := log.NewLogfmtLogger(w)

	labels := "app.kubernetes.io/name=open-ondemand"
	namespaceLabels = &labels

	timeNow = func() time.Time {
		t, _ := time.Parse("01/02/2006 15:04:05", "01/01/2020 15:00:00")
		return t
	}

	namespaces, err := getNamespaces(clientset, logger)
	namespaceLabels = &noString
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	jobs, err := GetJobs(clientset, namespaces, logger)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(jobs) != 1 {
		t.Errorf("Expected 1 jobs, got %d", len(jobs))
		return
	}
	if val := jobs[0].jobID; val != "1" {
		t.Errorf("Unexpected jobID, got: %v", val)
	}
}

func TestRun(t *testing.T) {
	if _, err := kingpin.CommandLine.Parse([]string{}); err != nil {
		t.Fatal(err)
	}
	w := log.NewSyncWriter(os.Stderr)
	logger := log.NewLogfmtLogger(w)

	labels := "app.kubernetes.io/managed-by=open-ondemand"
	podsLabels = &labels

	timeNow = func() time.Time {
		t, _ := time.Parse("01/02/2006 15:04:05", "01/01/2020 15:00:00")
		return t
	}

	run(clientset, logger)

	pods, err := clientset.CoreV1().Pods(metav1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		t.Errorf("Unexpected error getting pods: %v", err)
	}
	if len(pods.Items) != 2 {
		t.Errorf("Unexpected number of pods, got: %d", len(pods.Items))
	}
	services, err := clientset.CoreV1().Services(metav1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		t.Errorf("Unexpected error getting services: %v", err)
	}
	if len(services.Items) != 0 {
		t.Errorf("Unexpected number of services, got: %d", len(services.Items))
	}
	configmaps, err := clientset.CoreV1().ConfigMaps(metav1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		t.Errorf("Unexpected error getting configmaps: %v", err)
	}
	if len(configmaps.Items) != 0 {
		t.Errorf("Unexpected number of services, got: %d", len(configmaps.Items))
	}
	secrets, err := clientset.CoreV1().Services(metav1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		t.Errorf("Unexpected error getting secrets: %v", err)
	}
	if len(secrets.Items) != 0 {
		t.Errorf("Unexpected number of services, got: %d", len(secrets.Items))
	}
}
