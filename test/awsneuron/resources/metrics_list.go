// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package resources

const (
	ContainerNeuronCoreUtil               = "container_neuroncore_utilization"
	ContainerNeuronCoreMemUsageConstants  = "container_neuroncore_memory_usage_constants"
	ContainerNeuronCoreMemUsageModel      = "container_neuroncore_memory_usage_model_code"
	ContainerNeuronCoreMemUsageScratchpad = "container_neuroncore_memory_usage_model_shared_scratchpad"
	ContainerNeuronCoreMemUsageRuntime    = "container_neuroncore_memory_usage_runtime_memory"
	ContainerNeuronCoreMemUsageTensors    = "container_neuroncore_memory_usage_tensors"
	ContainerNeuronCoreMemUsageTotal      = "container_neuroncore_memory_usage_total"
	ContainerNeuronDeviceHwEccEvents      = "container_neurondevice_hw_ecc_events_total"

	PodNeuronCoreUtil               = "pod_neuroncore_utilization"
	PodNeuronCoreMemUsageConstants  = "pod_neuroncore_memory_usage_constants"
	PodNeuronCoreMemUsageModel      = "pod_neuroncore_memory_usage_model_code"
	PodNeuronCoreMemUsageScratchpad = "pod_neuroncore_memory_usage_model_shared_scratchpad"
	PodNeuronCoreMemUsageRuntime    = "pod_neuroncore_memory_usage_runtime_memory"
	PodNeuronCoreMemUsageTensors    = "pod_neuroncore_memory_usage_tensors"
	PodNeuronCoreMemUsageTotal      = "pod_neuroncore_memory_usage_total"
	PodNeuronDeviceHwEccEvents      = "pod_neurondevice_hw_ecc_events_total"

	NodeNeuronCoreUtil                     = "node_neuroncore_utilization"
	NodeNeuronCoreMemUsageConstants        = "node_neuroncore_memory_usage_constants"
	NodeNeuronCoreMemUsageModel            = "node_neuroncore_memory_usage_model_code"
	NodeNeuronCoreMemUsageScratchpad       = "node_neuroncore_memory_usage_model_shared_scratchpad"
	NodeNeuronCoreMemUsageRuntime          = "node_neuroncore_memory_usage_runtime_memory"
	NodeNeuronCoreMemUsageTensors          = "node_neuroncore_memory_usage_tensors"
	NodeNeuronCoreMemUsageTotal            = "node_neuroncore_memory_usage_total"
	NodeNeuronDeviceHwEccEvents            = "node_neurondevice_hw_ecc_events_total"
	NodeExecutionErrorsTotal               = "node_neuron_execution_errors_total"
	NodeExecutionErrorsGeneric             = "node_neuron_execution_errors_generic"
	NodeExecutionErrorsNumerical           = "node_neuron_execution_errors_numerical"
	NodeExecutionErrorsTransient           = "node_neuron_execution_errors_transient"
	NodeExecutionErrorsModel               = "node_neuron_execution_errors_model"
	NodeExecutionErrorsRuntime             = "node_neuron_execution_errors_runtime"
	NodeExecutionErrorsHardware            = "node_neuron_execution_errors_hardware"
	NodeExecutionStatusCompleted           = "node_neuron_execution_status_completed"
	NodeExecutionStatusTimedOut            = "node_neuron_execution_status_timed_out"
	NodeExecutionStatusCompletedWithErr    = "node_neuron_execution_status_completed_with_err"
	NodeExecutionStatusCompletedWithNumErr = "node_neuron_execution_status_completed_with_num_err"
	NodeExecutionStatusIncorrectInput      = "node_neuron_execution_status_incorrect_input"
	NodeExecutionStatusFailedToQueue       = "node_neuron_execution_status_failed_to_queue"
	NodeNeuronDeviceRuntimeMemoryUsed      = "node_neurondevice_runtime_memory_used_bytes"
	NodeNeuronExecutionLatency             = "node_neuron_execution_latency"
)
