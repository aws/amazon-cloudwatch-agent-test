package main

import (
	"flag"
	"log"
	"strings"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/shirou/gopsutil/v3/process"
)

var (
	cpuLimit = flag.Int("cpuLimit", 1, "agent's upper cpu usage limit in percent")
	memLimit = flag.Int("memLimit", 200000000, "agent's upper memory usage limit in Bytes")
	interval = flag.Duration("interval", 10*time.Second, "how frequently to validate")
	testName = flag.String("testName", "SoakTestLinux", "namespace for metrics")
)

// main() runs forever.
// It checks the resource usage of the agent process and reports metrics to CloudWatch.
// Intentionally not relying on the agent to check itself.
func main() {
	flag.Parse()
	log.Printf("Start validating, cpuLimit %d, memLimit %d MiB", *cpuLimit, *memLimit)
	// Sleep to avoid alarming on CWA using more resources during start up.
	time.Sleep(time.Minute)
	ticker := time.NewTicker(*interval)
	for range ticker.C {
		p := getAgentProcess()
		if p == nil {
			log.Printf("error: agent process not found")
			awsservice.ReportMetric(*testName, "FailCount", 1, types.StandardUnitCount)
			continue
		}
		// todo: check for unexpected crash/restart (createtime change).
		validate("AgentCpu", getCpuUsage(p), float64(*cpuLimit))
		validate("AgentMemory", getMem(p), float64(*memLimit))
		// todo: permission denied trying to get numFD, may need to with sudo.
		//validate("AgentNumFD", getNumFDs(p), 100)
		validate("AgentNumThreads", getNumThreads(p), 100)
	}
}

// Get agent process.
func getAgentProcess() *process.Process {
	procs, err := process.Processes()
	if err != nil {
		return nil
	}
	for _, p := range procs {
		c, _ := p.Cmdline()
		if strings.Contains(c, "amazon-cloudwatch-agent") {
			return p
		}
	}
	return nil
}

func validate(metricName string, value float64, limit float64) {
	log.Printf("validating, %s, %.1f, %.1f", metricName, value, limit)
	// Report actual usage for easier debugging.
	awsservice.ReportMetric(*testName, metricName, value, types.StandardUnitCount)
	// Report test failures with a common metric name.
	var fail float64 = 0
	if value > limit {
		fail = 1
	}
	// Always report the metric even if it isn't failing to avoid sparse metrics.
	awsservice.ReportMetric(*testName, "FailCount", fail, types.StandardUnitCount)
}

func getCpuUsage(p *process.Process) float64 {
	cp, err := p.CPUPercent()
	if err != nil {
		log.Printf("error: cpu, %v", err)
		// Return something that will breach threshold.
		return 200
	}
	return cp
}

func getMem(p *process.Process) float64 {
	mi, err := p.MemoryInfo()
	if err != nil {
		log.Printf("error: mem, %v", err)
		// Return something that will breach threshold.
		return 999 * 1024 * 1024
	}
	return float64(mi.RSS)
}

func getNumFDs(p *process.Process) float64 {
	num, err := p.NumFDs()
	if err != nil {
		log.Printf("error: numFDs, %v", err)
		return 100_000
	}
	return float64(num)
}

func getNumThreads(p *process.Process) float64 {
	num, err := p.NumThreads()
	if err != nil {
		log.Printf("error: NumThreads, %v", err)
		return 100_000
	}
	return float64(num)
}
