package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

type TestConfig struct {
	metricTests []func(*testing.T)
}

var testRegistry = map[string]TestConfig{
	"jvm_tomcat.json": {
		metricTests: []func(*testing.T){
			testTomcatMetrics,
			testTomcatSessions,
		},
	},
	"kafka.json": {
		metricTests: []func(*testing.T){
			testKafkaMetrics,
		},
	},
	"containerinsights.json": {
		metricTests: []func(*testing.T){},
	},
}

func TestMain(m *testing.M) {
	flag.Parse()
	env := environment.GetEnvironmentMetaData()

	if env.Region != "us-west-2" {
		if err := awsservice.ReconfigureAWSClients(env.Region); err != nil {
			fmt.Printf("Failed to reconfigure AWS clients: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("AWS clients reconfigured to use region: %s\n", env.Region)
	} else {
		fmt.Printf("Using default testing region: us-west-2\n")
	}

	fmt.Println("Starting Helm installation...")
	if err := ApplyHelm(env); err != nil {
		fmt.Printf("Failed to apply Helm: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Waiting for metrics to propagate...")
	time.Sleep(5 * time.Minute)

	os.Exit(m.Run())
}

// Make this a utility function.
func ApplyHelm(env *environment.MetaData) error {
	updateKubeconfig := exec.Command("aws", "eks", "update-kubeconfig", "--name", env.EKSClusterName)
	output, err := updateKubeconfig.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to update kubeconfig: %w\nOutput: %s", err, output)
	}

	helm := []string{
		"helm", "upgrade", "--install", "amazon-cloudwatch-observability",
		filepath.Join("..", "..", "..", "terraform", "eks", "e2e", "helm-charts", "charts", "amazon-cloudwatch-observability"),
		"--values", filepath.Join("..", "..", "..", "terraform", "eks", "e2e", "helm-charts", "charts", "amazon-cloudwatch-observability", "values.yaml"),
		"--set", fmt.Sprintf("clusterName=%s", env.EKSClusterName),
		"--set", fmt.Sprintf("region=%s", env.Region),
		"--set", fmt.Sprintf("agent.image.repository=%s", env.CloudwatchAgentRepository),
		"--set", fmt.Sprintf("agent.image.tag=%s", env.CloudwatchAgentTag),
		"--set", fmt.Sprintf("agent.image.repositoryDomainMap.public=%s", env.CloudwatchAgentRepositoryURL),
		"--set", fmt.Sprintf("manager.image.repository=%s", env.CloudwatchAgentOperatorRepository),
		"--set", fmt.Sprintf("manager.image.tag=%s", env.CloudwatchAgentOperatorTag),
		"--set", fmt.Sprintf("manager.image.repositoryDomainMap.public=%s", env.CloudwatchAgentOperatorRepositoryURL),
		"--namespace", "amazon-cloudwatch",
		"--create-namespace",
	}

	if env.AgentConfig != "" {
		agentConfigContent, err := os.ReadFile(env.AgentConfig)
		if err != nil {
			return fmt.Errorf("failed to read agent config file: %w", err)
		}
		helm = append(helm, "--set-json", fmt.Sprintf("agent.config=%s", string(agentConfigContent)))
	}

	helmUpgrade := exec.Command(helm[0], helm[1:]...)
	helmUpgrade.Stdout = os.Stdout
	helmUpgrade.Stderr = os.Stderr
	if err := helmUpgrade.Run(); err != nil {
		return fmt.Errorf("failed to install Helm release: %w", err)
	}

	fmt.Println("Waiting for CloudWatch Agent Operator to initialize...")
	time.Sleep(300 * time.Second)

	deploymentName := strings.TrimSuffix(filepath.Base(env.SampleApp), ".yml")

	apply := exec.Command("kubectl", "apply", "-f", env.SampleApp)
	output, err = apply.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to apply sample app: %w\nOutput: %s", err, output)
	}

	wait := exec.Command("kubectl", "wait", "--for=condition=available", "--timeout=300s", fmt.Sprintf("deployment/%s", deploymentName), "-n", "default")
	output, err = wait.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to wait for deployment %s: %w\nOutput: %s", deploymentName, err, output)
	}

	var ports string
	if deploymentName == "tomcat-deployment" {
		ports = "8080:8080"
	} else if deploymentName == "kafka-deployment" {
		ports = "9092:9092"
	} else {
		return fmt.Errorf("unknown deployment type: %s", deploymentName)
	}

	portForward := exec.Command("kubectl", "port-forward", fmt.Sprintf("deployment/%s", deploymentName), ports)
	portForward.Stdout = os.Stdout
	portForward.Stderr = os.Stderr
	if err := portForward.Run(); err != nil {
		fmt.Printf("Port forwarding failed: %v\n", err)
	}

	return nil
}

func TestResources(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", filepath.Join(os.Getenv("HOME"), ".kube", "config"))
	if err != nil {
		t.Fatalf("Error building kubeconfig: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatalf("Error creating Kubernetes client: %v", err)
	}

	daemonSet, err := clientset.AppsV1().DaemonSets("amazon-cloudwatch").Get(context.TODO(), "cloudwatch-agent", metav1.GetOptions{})
	if err != nil {
		t.Errorf("Error getting CloudWatch Agent DaemonSet: %v", err)
	}
	if daemonSet == nil {
		t.Error("CloudWatch Agent DaemonSet not found")
	}

	configMap, err := clientset.CoreV1().ConfigMaps("amazon-cloudwatch").Get(context.TODO(), "cloudwatch-agent", metav1.GetOptions{})
	if err != nil {
		t.Errorf("Error getting CloudWatch Agent ConfigMap: %v", err)
	}
	if configMap == nil {
		t.Error("CloudWatch Agent ConfigMap not found")
	}
}

func TestMetrics(t *testing.T) {
	env := environment.GetEnvironmentMetaData()
	configFile := filepath.Base(env.AgentConfig)

	config, exists := testRegistry[configFile]
	if !exists {
		t.Skipf("No tests registered for config file: %s", configFile)
		return
	}

	for _, testFunc := range config.metricTests {
		testFunc(t)
	}
}

func testTomcatMetrics(t *testing.T) {
	t.Run("verify_jvm_tomcat_metrics", func(t *testing.T) {
		metricsToCheck := []struct {
			name      string
			namespace string
		}{
			{"tomcat.traffic", "JVM_TOMCAT_E2E"},
			{"jvm.classes.loaded", "JVM_TOMCAT_E2E"},
			{"jvm.gc.collections.count", "JVM_TOMCAT_E2E"},
			{"jvm.gc.collections.elapsed", "JVM_TOMCAT_E2E"},
			{"jvm.memory.heap.init", "JVM_TOMCAT_E2E"},
			{"jvm.memory.heap.max", "JVM_TOMCAT_E2E"},
			{"jvm.memory.heap.used", "JVM_TOMCAT_E2E"},
			{"jvm.memory.heap.committed", "JVM_TOMCAT_E2E"},
			{"jvm.memory.nonheap.init", "JVM_TOMCAT_E2E"},
			{"jvm.memory.nonheap.max", "JVM_TOMCAT_E2E"},
			{"jvm.memory.nonheap.used", "JVM_TOMCAT_E2E"},
			{"jvm.memory.nonheap.committed", "JVM_TOMCAT_E2E"},
			{"jvm.memory.pool.init", "JVM_TOMCAT_E2E"},
			{"jvm.memory.pool.max", "JVM_TOMCAT_E2E"},
			{"jvm.memory.pool.used", "JVM_TOMCAT_E2E"},
			{"jvm.memory.pool.committed", "JVM_TOMCAT_E2E"},
			{"jvm.threads.count", "JVM_TOMCAT_E2E"},
			{"tomcat.sessions", "JVM_TOMCAT_E2E"},
			{"tomcat.errors", "JVM_TOMCAT_E2E"},
			{"tomcat.request_count", "JVM_TOMCAT_E2E"},
			{"tomcat.max_time", "JVM_TOMCAT_E2E"},
			{"tomcat.processing_time", "JVM_TOMCAT_E2E"},
			{"tomcat.threads", "JVM_TOMCAT_E2E"},
		}

		for _, metric := range metricsToCheck {
			t.Run(metric.name, func(t *testing.T) {
				awsservice.ValidateMetricWithTest(t, metric.name, metric.namespace, nil, 5, 1*time.Minute)
			})
		}
	})
}

func testTomcatSessions(t *testing.T) {
	t.Run("verify_tomcat_sessions", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			resp, err := http.Get("http://localhost:8080/webapp/index.jsp")
			if err != nil {
				t.Logf("Request attempt %d failed: %v", i+1, err)
				continue
			}
			err = resp.Body.Close()
			if err != nil {
				t.Errorf("Failed to close response body: %v", err)
				return
			}
		}

		time.Sleep(30 * time.Second)

		startTime := time.Now().Add(-5 * time.Minute)
		endTime := time.Now()

		hasActiveSessions := awsservice.ValidateSampleCount(
			"tomcat.sessions",
			"JVM_TOMCAT_E2E",
			nil,
			startTime,
			endTime,
			1,
			1000,
			60,
		)

		if !hasActiveSessions {
			t.Error("Expected non-zero tomcat.sessions after applying traffic")
		}
	})
}

func testKafkaMetrics(t *testing.T) {
	t.Run("verify_kafka_metrics", func(t *testing.T) {
		metricsToCheck := []struct {
			name      string
			namespace string
		}{
			{"kafka.message.count", "KAFKA_E2E"},
			{"kafka.request.count", "KAFKA_E2E"},
			{"kafka.request.failed", "KAFKA_E2E"},
			{"kafka.request.time.total", "KAFKA_E2E"},
			{"kafka.request.time.50p", "KAFKA_E2E"},
			{"kafka.request.time.99p", "KAFKA_E2E"},
			{"kafka.request.time.avg", "KAFKA_E2E"},
			{"kafka.network.io", "KAFKA_E2E"},
			{"kafka.purgatory.size", "KAFKA_E2E"},
			{"kafka.partition.count", "KAFKA_E2E"},
			{"kafka.partition.offline", "KAFKA_E2E"},
			{"kafka.partition.under_replicated", "KAFKA_E2E"},
			{"kafka.isr.operation.count", "KAFKA_E2E"},
			{"kafka.max.lag", "KAFKA_E2E"},
			{"kafka.controller.active.count", "KAFKA_E2E"},
			{"kafka.leader.election.rate", "KAFKA_E2E"},
			{"kafka.unclean.election.rate", "KAFKA_E2E"},
			{"kafka.request.queue", "KAFKA_E2E"},
			{"kafka.logs.flush.time.count", "KAFKA_E2E"},
			{"kafka.logs.flush.time.median", "KAFKA_E2E"},
			{"kafka.logs.flush.time.99p", "KAFKA_E2E"},
			{"kafka.consumer.fetch-rate", "KAFKA_E2E"},
			{"kafka.consumer.records-lag-max", "KAFKA_E2E"},
			{"kafka.consumer.total.bytes-consumed-rate", "KAFKA_E2E"},
			{"kafka.consumer.total.fetch-size-avg", "KAFKA_E2E"},
			{"kafka.consumer.total.records-consumed-rate", "KAFKA_E2E"},
			{"kafka.consumer.bytes-consumed-rate", "KAFKA_E2E"},
			{"kafka.consumer.fetch-size-avg", "KAFKA_E2E"},
			{"kafka.consumer.records-consumed-rate", "KAFKA_E2E"},
			{"kafka.producer.io-wait-time-ns-avg", "KAFKA_E2E"},
			{"kafka.producer.outgoing-byte-rate", "KAFKA_E2E"},
			{"kafka.producer.request-latency-avg", "KAFKA_E2E"},
			{"kafka.producer.request-rate", "KAFKA_E2E"},
			{"kafka.producer.response-rate", "KAFKA_E2E"},
			{"kafka.producer.byte-rate", "KAFKA_E2E"},
			{"kafka.producer.compression-rate", "KAFKA_E2E"},
			{"kafka.producer.record-error-rate", "KAFKA_E2E"},
			{"kafka.producer.record-retry-rate", "KAFKA_E2E"},
			{"kafka.producer.record-send-rate", "KAFKA_E2E"},
		}

		for _, metric := range metricsToCheck {
			t.Run(metric.name, func(t *testing.T) {
				awsservice.ValidateMetricWithTest(t, metric.name, metric.namespace, nil, 5, 1*time.Minute)
			})
		}
	})
}

// Implement order and also helm upgrade + metric test.
