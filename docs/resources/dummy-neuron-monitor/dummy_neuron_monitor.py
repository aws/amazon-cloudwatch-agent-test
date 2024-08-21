#!/usr/bin/env python3

import sys
import json
import argparse
import signal
import hashlib
import time
from prometheus_client import start_http_server, Gauge, Counter, Info


def get_instance_labels(instance_info):
    instance_labels = {
        'instance_name': instance_info['instance_name'],
        'instance_id': instance_info['instance_id'],
        'instance_type': instance_info['instance_type'],
        'availability_zone': instance_info['instance_availability_zone'],
        'region': instance_info['instance_region'],
        'subnet_id': instance_info['subnet_id']
    }
    return instance_labels


def get_runtime_labels(instance_info, runtime_tag):
    label_dict = instance_info.copy()
    label_dict['runtime_tag'] = runtime_tag
    return label_dict


def process_neuroncore_counters(group_obj, data, labels):
    gauge_name = 'neuroncore_utilization_ratio'
    labels['neuroncore'] = None
    if gauge_name not in group_obj:
        group_obj[gauge_name] = Gauge(gauge_name, 'NeuronCore utilization ratio', labels.keys())
    for nc_idx, nc_data in data['neuroncores_in_use'].items():
        labels['neuroncore'] = int(nc_idx)
        group_obj[gauge_name].labels(**labels).set(nc_data['neuroncore_utilization'] / 100.0)


def process_neuron_runtime_vcpu_usage(group_obj, data, labels):
    gauge_name = 'neuron_runtime_vcpu_usage_ratio'
    labels['usage_type'] = None
    if gauge_name not in group_obj:
        group_obj[gauge_name] = Gauge(gauge_name, 'Runtime vCPU utilization ratio', labels.keys())
    cpu_usage_fields = ['user', 'system']
    for field in cpu_usage_fields:
        labels['usage_type'] = field
        group_obj[gauge_name].labels(**labels).set(data['vcpu_usage'][field] / 100.0)


def process_memory_used(group_obj, data, labels):
    gauge_name = 'neuron_runtime_memory_used_bytes'
    labels['memory_location'] = None
    if gauge_name not in group_obj:
        group_obj[gauge_name] = Gauge(gauge_name, 'Runtime memory used bytes', labels.keys())
    mem_locations = ['host', 'neuron_device']
    for mem_location_type in mem_locations:
        labels['memory_location'] = mem_location_type
        group_obj[gauge_name].labels(**labels).set(data['neuron_runtime_used_bytes'][mem_location_type])

    gauge_name_prefix = 'neuroncore_memory_usage_{}'
    labels['neuroncore'] = None
    labels['memory_location'] = None
    neuroncore_memory_usage_type = ['constants', 'model_code', 'model_shared_scratchpad', 'runtime_memory', 'tensors']
    for memory_usage_type in neuroncore_memory_usage_type:
        gauge_name = gauge_name_prefix.format(memory_usage_type)
        if gauge_name not in group_obj:
            group_obj[gauge_name] = Gauge(gauge_name, 'NeuronCore memory utilization for {}'.format(memory_usage_type), labels.keys())
        for nc_idx, nc_data in data['neuron_runtime_used_bytes']['usage_breakdown']['neuroncore_memory_usage'].items():
            labels['neuroncore'] = int(nc_idx)
            group_obj[gauge_name].labels(**labels).set(nc_data[memory_usage_type])


def process_execution_stats(group_obj, data, labels):
    counter_name = 'execution_errors_total'
    err_labels = labels.copy()
    err_labels['error_type'] = None
    if counter_name not in group_obj:
        group_obj[counter_name] = Counter(counter_name, 'Execution errors total', err_labels.keys())
    error_summary = data['error_summary']
    for error_type in error_summary:
        err_labels['error_type'] = error_type
        group_obj[counter_name].labels(**err_labels).inc(error_summary[error_type])

    counter_name = 'execution_status_total'
    status_labels = labels.copy()
    status_labels['status_type'] = None
    if counter_name not in group_obj:
        group_obj[counter_name] = Counter(counter_name, 'Execution status total', status_labels.keys())
    execution_summary = data['execution_summary']
    for execution_outcome in execution_summary:
        status_labels['status_type'] = execution_outcome
        group_obj[counter_name].labels(**status_labels).inc(execution_summary[execution_outcome])

    gauge_name = 'execution_latency_seconds'
    latency_labels = labels.copy()
    latency_labels['percentile'] = None
    if gauge_name not in group_obj:
        group_obj[gauge_name] = Gauge(gauge_name, 'Execution latency in seconds', latency_labels.keys())
    latency_stats = data['latency_stats']
    if latency_stats['total_latency'] is not None:
        for percentile in latency_stats['total_latency']:
            latency_labels['percentile'] = percentile
            group_obj[gauge_name].labels(**latency_labels).set(latency_stats['total_latency'][percentile])


def process_neuron_hw_counters(group_obj, data, labels):
    counter_name = 'hardware_ecc_events_total'
    labels['event_type'] = None
    labels['neuron_device_index'] = None
    if counter_name not in group_obj:
        group_obj[counter_name] = Counter(counter_name, 'Hardware ecc events total', labels.keys())
    hw_counters = ['mem_ecc_corrected', 'mem_ecc_uncorrected', 'sram_ecc_corrected', 'sram_ecc_uncorrected']
    for device in data['neuron_devices']:
        for counter in hw_counters:
            labels['event_type'] = counter
            labels['neuron_device_index'] = device['neuron_device_index']
            group_obj[counter_name].labels(**labels).inc(device[counter])


def process_vcpu_usage(group_obj, data, labels):
    cpu_usage_aggregation = {
        'user': ['user', 'nice'],
        'system': ['system', 'io_wait', 'irq', 'soft_irq']
    }
    gauge_name = 'system_vcpu_count'
    if gauge_name not in group_obj:
        group_obj[gauge_name] = Gauge(gauge_name, 'System vCPU count', labels.keys())
    group_obj[gauge_name].labels(**labels).set(len(data['usage_data']))

    labels['usage_type'] = None
    gauge_name = 'system_vcpu_usage_ratio'
    if gauge_name not in group_obj:
        group_obj[gauge_name] = Gauge(gauge_name, 'System CPU utilization ratio', labels.keys())
    for field, aggregated in cpu_usage_aggregation.items():
        aggregate_value = sum([data['average_usage'][item] for item in aggregated])
        aggregate_value = min(aggregate_value, 100.0)
        labels['usage_type'] = field
        group_obj[gauge_name].labels(**labels).set(aggregate_value / 100.0)


def process_memory_info(group_obj, data, labels):
    for entries in [('memory', 'system_memory'), ('swap', 'system_swap')]:
        for stat in ['total_bytes', 'used_bytes']:
            gauge_name = '{}_{}'.format(entries[1], stat)
            if gauge_name not in group_obj:
                group_obj[gauge_name] = Gauge(gauge_name,
                                              'System {} {} bytes'.format(entries[0], stat), labels.keys())
            src_entry = '{}_{}'.format(entries[0], stat)
            group_obj[gauge_name].labels(**labels).set(data[src_entry])


def process_neuron_hardware_info(metric_objects, data, instance_data):
    if 'neuron_hardware_info' not in metric_objects:
        neuron_labels = {
            'neuron_device_count': str(data['neuron_device_count']),
            'neuroncore_per_device_count': str(data['neuroncore_per_device_count'])
        }
        neuron_labels.update(instance_data)

        metric_objects['neuron_hardware_info'] = Info('neuron_hardware', 'Neuron Hardware Information')
        metric_objects['neuron_hardware_info'].info(neuron_labels)


def process_instance_info(metric_objects, instance_data):
    if 'instance_info' not in metric_objects:
        metric_objects['instance_info'] = Info('instance', 'EC2 instance information')
        metric_objects['instance_info'].info(instance_data)


def process_report_entries(metric_objects, report_entries, labels, runtime_tag=None):
    for metric_group_name, metric_group_data in report_entries.items():
        handler_name = 'process_{}'.format(metric_group_name)
        if handler_name in globals():
            crt_error = metric_group_data['error']
            if crt_error == '':
                if metric_group_name not in metric_objects:
                    metric_objects[metric_group_name] = {}
                metric_group_object = metric_objects[metric_group_name]
                globals()[handler_name](metric_group_object, metric_group_data, labels.copy())
            else:
                if runtime_tag is not None:
                    print('Error getting {} for runtime tag {}: {}'.format(
                        metric_group_name, runtime_tag, crt_error), file=sys.stderr)
                else:
                    print('Error getting {}: {}'.format(metric_group_name, crt_error), file=sys.stderr)


def process_data(metric_objects, monitor_data, instance_info):
    if monitor_data.get('neuron_runtime_data', []):
        for runtime in monitor_data['neuron_runtime_data']:
            runtime_tag = runtime['neuron_runtime_tag']

            if runtime['error'] != '':
                print('Runtime {} error: {}'.format(runtime_tag, runtime['error']), file=sys.stderr)
                continue

            process_report_entries(metric_objects, runtime['report'],
                                   get_runtime_labels(instance_info, runtime_tag), runtime_tag)
    else: # Reset gauges if no nueron_runtime is running
        clear_gauges_from_metric_objects(metric_objects)

    if monitor_data['system_data'] is not None:
        process_report_entries(metric_objects, monitor_data['system_data'], instance_info)
    process_instance_info(metric_objects, instance_info)
    if monitor_data['neuron_hardware_info'] is not None:
        process_neuron_hardware_info(metric_objects, monitor_data['neuron_hardware_info'], instance_info)

def clear_gauges_from_metric_objects(all_metric_objects):
    for _, metricGroupedObjects in all_metric_objects.items():
        if(isinstance(metricGroupedObjects,dict)):
            for _, metrics in metricGroupedObjects.items():
                if metrics._type == 'gauge' or metrics._type == 'counter':
                    metrics._metrics.clear()

def _calculate_file_hash(file_path):
    with open(file_path, "rb") as f:
        file_hash = hashlib.sha256(f.read()).hexdigest()
    return file_hash

def _update_ssl_cxt(certfile, keyfile):
    global ssl_cxt
    ssl_cxt.load_cert_chain(certfile=certfile, keyfile=keyfile)
    print("Refreshing TLS certificates")

def _watch_file_and_update_ssl_cxt(original_hash, certfile, keyfile):
    current_certfile_hash = _calculate_file_hash(certfile)
    current_keyfile_hash = _calculate_file_hash(keyfile)
    if (original_hash["certfile"] != current_certfile_hash) or (original_hash["keyfile"] != current_keyfile_hash):
        _update_ssl_cxt(certfile, keyfile)
        original_hash["certfile"] = current_certfile_hash
        original_hash["keyfile"] = current_keyfile_hash

def update_loop(certfile, keyfile):
    running = True

    def signal_handler(*_):
        nonlocal running
        running = False
    signal.signal(signal.SIGINT, signal_handler)

    """ Dictionary containing all prometheus client objects, first by metric group and
        then by metric, for example, for neuroncore_counters:
        all_metric_objects['neuroncore_counters']['neuroncore_utilization_ratio'] = Gauge(...)
    """
    all_metric_objects = {}
    original_file_hash = {}
    instance_labels = None
    if certfile and keyfile:
        certfile_hash = _calculate_file_hash(certfile)
        keyfile_hash = _calculate_file_hash(keyfile)
        original_file_hash = {"certfile":certfile_hash,"keyfile":keyfile_hash}

    while running:
        line = ('{"neuron_runtime_data":[{"pid":457402,"neuron_runtime_tag":"367","error":"","report":{'
                '"execution_stats":{"period":4.999666547,"error_summary":{"generic":2,"numerical":0,"transient":0,'
                '"model":0,"runtime":0,"hardware":0},"execution_summary":{"completed":2,"completed_with_err":0,'
                '"completed_with_num_err":0,"timed_out":0,"incorrect_input":0,"failed_to_queue":0},"latency_stats":{'
                '"total_latency":null,"device_latency":null},"error":""},"memory_used":{"period":4.999671285,'
                '"neuron_runtime_used_bytes":{"host":9043968,"neuron_device":3541303936,"usage_breakdown":{"host":{'
                '"application_memory":655360,"constants":0,"dma_buffers":8388608,"tensors":0},'
                '"neuroncore_memory_usage":{"0":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,'
                '"runtime_memory":0,"tensors":9912852},"1":{"constants":0,"model_code":100752896,'
                '"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},"2":{"constants":0,'
                '"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},'
                '"3":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,'
                '"tensors":9912852},"4":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,'
                '"runtime_memory":0,"tensors":9912852},"5":{"constants":0,"model_code":100752896,'
                '"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},"6":{"constants":0,'
                '"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},'
                '"7":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,'
                '"tensors":9912852},"8":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,'
                '"runtime_memory":0,"tensors":9912852},"9":{"constants":0,"model_code":100752896,'
                '"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},"10":{"constants":0,'
                '"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},'
                '"11":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,'
                '"tensors":9912852},"12":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,'
                '"runtime_memory":0,"tensors":9912852},"13":{"constants":0,"model_code":100752896,'
                '"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},"14":{"constants":0,'
                '"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},'
                '"15":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,'
                '"tensors":9912852},"16":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,'
                '"runtime_memory":0,"tensors":9912852},"17":{"constants":0,"model_code":100752896,'
                '"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},"18":{"constants":0,'
                '"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},'
                '"19":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,'
                '"tensors":9912852},"20":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,'
                '"runtime_memory":0,"tensors":9912852},"21":{"constants":0,"model_code":100752896,'
                '"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},"22":{"constants":0,'
                '"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},'
                '"23":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,'
                '"tensors":9912852},"24":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,'
                '"runtime_memory":0,"tensors":9912852},"25":{"constants":0,"model_code":100752896,'
                '"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},"26":{"constants":0,'
                '"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},'
                '"27":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,'
                '"tensors":9912852},"28":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,'
                '"runtime_memory":0,"tensors":9912852},"29":{"constants":0,"model_code":100752896,'
                '"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},"30":{"constants":0,'
                '"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},'
                '"31":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,'
                '"tensors":9912852}}}},"loaded_models":[{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10019,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":5}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10005,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":5}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10007,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":10}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10013,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":10}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10029,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":2}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10032,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":2}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10004,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":0}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10012,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":0}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10001,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":3}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10016,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":3}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10022,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":11}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10024,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":11}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10026,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":13}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10031,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":13}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10025,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":15}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10021,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":15}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10006,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":12}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10011,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":12}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10010,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":6}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10015,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":6}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10008,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":1}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10027,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":1}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10018,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":14}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10030,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":14}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10017,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":9}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10003,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":9}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10002,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":4}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10009,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":4}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10020,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":7}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10028,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":7}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10014,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":8}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10023,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":8}}}],"error":""},"neuron_runtime_vcpu_usage":{"period":4.999647382,'
                '"vcpu_usage":{"user":0,"system":0},"error":"open/proc/457402/stat:nosuchfileordirectory"},'
                '"neuroncore_counters":{"period":4.999667932,"neuroncores_in_use":{"0":{"neuroncore_utilization":0},'
                '"1":{"neuroncore_utilization":0},"2":{"neuroncore_utilization":0},"3":{"neuroncore_utilization":0},'
                '"4":{"neuroncore_utilization":0},"5":{"neuroncore_utilization":0},"6":{"neuroncore_utilization": 41.4},'
                '"7":{"neuroncore_utilization":0},"8":{"neuroncore_utilization":0},"9":{"neuroncore_utilization":0},'
                '"10":{"neuroncore_utilization":0},"11":{"neuroncore_utilization":0},'
                '"12":{"neuroncore_utilization":0},"13":{"neuroncore_utilization":0},'
                '"14":{"neuroncore_utilization":0},"15":{"neuroncore_utilization":0},'
                '"16":{"neuroncore_utilization":0},"17":{"neuroncore_utilization":0},'
                '"18":{"neuroncore_utilization":0},"19":{"neuroncore_utilization":0},'
                '"20":{"neuroncore_utilization":0},"21":{"neuroncore_utilization":0},'
                '"22":{"neuroncore_utilization":0},"23":{"neuroncore_utilization":0},'
                '"24":{"neuroncore_utilization":0},"25":{"neuroncore_utilization":0},'
                '"26":{"neuroncore_utilization":0},"27":{"neuroncore_utilization":0},'
                '"28":{"neuroncore_utilization":0},"29":{"neuroncore_utilization":0},'
                '"30":{"neuroncore_utilization":0},"31":{"neuroncore_utilization":0}},"error":""}}}, {"pid":457402,"neuron_runtime_tag":"123","error":"","report":{'
                '"execution_stats":{"period":4.999666547,"error_summary":{"generic":2,"numerical":0,"transient":0,'
                '"model":0,"runtime":0,"hardware":0},"execution_summary":{"completed":2,"completed_with_err":0,'
                '"completed_with_num_err":0,"timed_out":0,"incorrect_input":0,"failed_to_queue":0},"latency_stats":{'
                '"total_latency":null,"device_latency":null},"error":""},"memory_used":{"period":4.999671285,'
                '"neuron_runtime_used_bytes":{"host":9043968,"neuron_device":3541303936,"usage_breakdown":{"host":{'
                '"application_memory":655360,"constants":0,"dma_buffers":8388608,"tensors":0},'
                '"neuroncore_memory_usage":{"0":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,'
                '"runtime_memory":0,"tensors":9912852},"1":{"constants":0,"model_code":100752896,'
                '"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},"2":{"constants":0,'
                '"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},'
                '"3":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,'
                '"tensors":9912852},"4":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,'
                '"runtime_memory":0,"tensors":9912852},"5":{"constants":0,"model_code":100752896,'
                '"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},"6":{"constants":0,'
                '"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},'
                '"7":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,'
                '"tensors":9912852},"8":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,'
                '"runtime_memory":0,"tensors":9912852},"9":{"constants":0,"model_code":100752896,'
                '"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},"10":{"constants":0,'
                '"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},'
                '"11":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,'
                '"tensors":9912852},"12":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,'
                '"runtime_memory":0,"tensors":9912852},"13":{"constants":0,"model_code":100752896,'
                '"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},"14":{"constants":0,'
                '"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},'
                '"15":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,'
                '"tensors":9912852},"16":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,'
                '"runtime_memory":0,"tensors":9912852},"17":{"constants":0,"model_code":100752896,'
                '"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},"18":{"constants":0,'
                '"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},'
                '"19":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,'
                '"tensors":9912852},"20":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,'
                '"runtime_memory":0,"tensors":9912852},"21":{"constants":0,"model_code":100752896,'
                '"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},"22":{"constants":0,'
                '"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},'
                '"23":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,'
                '"tensors":9912852},"24":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,'
                '"runtime_memory":0,"tensors":9912852},"25":{"constants":0,"model_code":100752896,'
                '"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},"26":{"constants":0,'
                '"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},'
                '"27":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,'
                '"tensors":9912852},"28":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,'
                '"runtime_memory":0,"tensors":9912852},"29":{"constants":0,"model_code":100752896,'
                '"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},"30":{"constants":0,'
                '"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,"tensors":9912852},'
                '"31":{"constants":0,"model_code":100752896,"model_shared_scratchpad":0,"runtime_memory":0,'
                '"tensors":9912852}}}},"loaded_models":[{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10019,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":5}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10005,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":5}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10007,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":10}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10013,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":10}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10029,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":2}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10032,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":2}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10004,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":0}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10012,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":0}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10001,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":3}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10016,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":3}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10022,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":11}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10024,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":11}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10026,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":13}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10031,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":13}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10025,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":15}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10021,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":15}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10006,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":12}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10011,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":12}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10010,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":6}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10015,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":6}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10008,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":1}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10027,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":1}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10018,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":14}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10030,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":14}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10017,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":9}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10003,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":9}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10002,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":4}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10009,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":4}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10020,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":7}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10028,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":7}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10014,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":1,'
                '"neuron_device_index":8}}},{"name":"2.2.0.73+0af5a171c-/neuronxcc-y_d15g2g",'
                '"uuid":"302e6ea0c77b11ee846d26b89fc7ffab","model_id":10023,"is_running":false,"subgraphs":{"sg_00":{'
                '"memory_used_bytes":{"host":20480,"neuron_device":24064,"usage_breakdown":{"host":{'
                '"application_memory":20480,"constants":0,"dma_buffers":0,"tensors":0},"neuron_device":{'
                '"constants":0,"model_code":24064,"runtime_memory":0,"tensors":0}}},"neuroncore_index":0,'
                '"neuron_device_index":8}}}],"error":""},"neuron_runtime_vcpu_usage":{"period":4.999647382,'
                '"vcpu_usage":{"user":0,"system":0},"error":"open/proc/457402/stat:nosuchfileordirectory"},'
                '"neuroncore_counters":{"period":4.999667932,"neuroncores_in_use":{"0":{"neuroncore_utilization":0},'
                '"1":{"neuroncore_utilization":0},"2":{"neuroncore_utilization":0},"3":{"neuroncore_utilization":0},'
                '"4":{"neuroncore_utilization":0},"5":{"neuroncore_utilization":0},"6":{"neuroncore_utilization":0},'
                '"7":{"neuroncore_utilization":0},"8":{"neuroncore_utilization":0},"9":{"neuroncore_utilization":0},'
                '"10":{"neuroncore_utilization":0},"11":{"neuroncore_utilization":0},'
                '"12":{"neuroncore_utilization":0},"13":{"neuroncore_utilization":0},'
                '"14":{"neuroncore_utilization":0},"15":{"neuroncore_utilization":0},'
                '"16":{"neuroncore_utilization":0},"17":{"neuroncore_utilization":0},'
                '"18":{"neuroncore_utilization":0},"19":{"neuroncore_utilization":0},'
                '"20":{"neuroncore_utilization":0},"21":{"neuroncore_utilization":0},'
                '"22":{"neuroncore_utilization":0},"23":{"neuroncore_utilization":0},'
                '"24":{"neuroncore_utilization":0},"25":{"neuroncore_utilization":0},'
                '"26":{"neuroncore_utilization":0},"27":{"neuroncore_utilization":0},'
                '"28":{"neuroncore_utilization":0},"29":{"neuroncore_utilization":0},'
                '"30":{"neuroncore_utilization":0},"31":{"neuroncore_utilization":0}},"error":""}}}],"system_data":{'
                '"memory_info":{"period":4.9997283150000005,"memory_total_bytes":532523487232,'
                '"memory_used_bytes":81207975936,"swap_total_bytes":0,"swap_used_bytes":0,"error":""},"vcpu_usage":{'
                '"period":4.999737702,"average_usage":{"user":19.66,"nice":0,"system":1.67,"idle":78.67,"io_wait":0,'
                '"irq":0,"soft_irq":0},"usage_data":{"0":{"user":51.31,"nice":0,"system":0,"idle":48.69,"io_wait":0,'
                '"irq":0,"soft_irq":0},"1":{"user":52.91,"nice":0,"system":6.21,"idle":40.88,"io_wait":0,"irq":0,'
                '"soft_irq":0},"2":{"user":25.6,"nice":0,"system":0,"idle":74.4,"io_wait":0,"irq":0,"soft_irq":0},'
                '"3":{"user":0.6,"nice":0,"system":0,"idle":99.4,"io_wait":0,"irq":0,"soft_irq":0},"4":{"user":0.2,'
                '"nice":0,"system":0,"idle":99.8,"io_wait":0,"irq":0,"soft_irq":0},"5":{"user":0.8,"nice":0,'
                '"system":0,"idle":99.2,"io_wait":0,"irq":0,"soft_irq":0},"6":{"user":1,"nice":0,"system":2.99,'
                '"idle":96.01,"io_wait":0,"irq":0,"soft_irq":0},"7":{"user":0,"nice":0,"system":0.2,"idle":99.8,'
                '"io_wait":0,"irq":0,"soft_irq":0},"8":{"user":1.8,"nice":0,"system":0,"idle":98.2,"io_wait":0,'
                '"irq":0,"soft_irq":0},"9":{"user":0.4,"nice":0,"system":0.8,"idle":98.8,"io_wait":0,"irq":0,'
                '"soft_irq":0},"10":{"user":0.2,"nice":0,"system":0,"idle":99.8,"io_wait":0,"irq":0,"soft_irq":0},'
                '"11":{"user":0,"nice":0,"system":0,"idle":100,"io_wait":0,"irq":0,"soft_irq":0},"12":{"user":0,'
                '"nice":0,"system":0,"idle":100,"io_wait":0,"irq":0,"soft_irq":0},"13":{"user":0,"nice":0,"system":0,'
                '"idle":100,"io_wait":0,"irq":0,"soft_irq":0},"14":{"user":0,"nice":0,"system":0.2,"idle":99.8,'
                '"io_wait":0,"irq":0,"soft_irq":0},"15":{"user":0.2,"nice":0,"system":0,"idle":99.8,"io_wait":0,'
                '"irq":0,"soft_irq":0},"16":{"user":0.2,"nice":0,"system":0,"idle":99.8,"io_wait":0,"irq":0,'
                '"soft_irq":0},"17":{"user":0.2,"nice":0,"system":0.2,"idle":99.6,"io_wait":0,"irq":0,"soft_irq":0},'
                '"18":{"user":0,"nice":0,"system":0,"idle":100,"io_wait":0,"irq":0,"soft_irq":0},"19":{"user":0,'
                '"nice":0,"system":0,"idle":100,"io_wait":0,"irq":0,"soft_irq":0},"20":{"user":0.4,"nice":0,'
                '"system":0.4,"idle":99.2,"io_wait":0,"irq":0,"soft_irq":0},"21":{"user":0.2,"nice":0,"system":0.2,'
                '"idle":99.6,"io_wait":0,"irq":0,"soft_irq":0},"22":{"user":0.2,"nice":0,"system":0,"idle":99.8,'
                '"io_wait":0,"irq":0,"soft_irq":0},"23":{"user":0,"nice":0,"system":0.2,"idle":99.8,"io_wait":0,'
                '"irq":0,"soft_irq":0},"24":{"user":0.2,"nice":0,"system":0.2,"idle":99.6,"io_wait":0,"irq":0,'
                '"soft_irq":0},"25":{"user":0,"nice":0,"system":0,"idle":100,"io_wait":0,"irq":0,"soft_irq":0},'
                '"26":{"user":0.2,"nice":0,"system":0.2,"idle":99.6,"io_wait":0,"irq":0,"soft_irq":0},'
                '"27":{"user":0.2,"nice":0,"system":0.4,"idle":99.4,"io_wait":0,"irq":0,"soft_irq":0},'
                '"28":{"user":0.2,"nice":0,"system":0.8,"idle":99,"io_wait":0,"irq":0,"soft_irq":0},"29":{"user":0.2,'
                '"nice":0,"system":0,"idle":99.8,"io_wait":0,"irq":0,"soft_irq":0},"30":{"user":0,"nice":0,'
                '"system":0.4,"idle":99.6,"io_wait":0,"irq":0,"soft_irq":0},"31":{"user":0,"nice":0,"system":0,'
                '"idle":100,"io_wait":0,"irq":0,"soft_irq":0},"32":{"user":0.2,"nice":0,"system":1,"idle":98.8,'
                '"io_wait":0,"irq":0,"soft_irq":0},"33":{"user":1.81,"nice":0,"system":4.44,"idle":93.75,"io_wait":0,'
                '"irq":0,"soft_irq":0},"34":{"user":5.22,"nice":0,"system":0.2,"idle":94.58,"io_wait":0,"irq":0,'
                '"soft_irq":0},"35":{"user":22,"nice":0,"system":0,"idle":78,"io_wait":0,"irq":0,"soft_irq":0},'
                '"36":{"user":47.31,"nice":0,"system":2.79,"idle":49.9,"io_wait":0,"irq":0,"soft_irq":0},'
                '"37":{"user":0.2,"nice":0,"system":0.2,"idle":99.6,"io_wait":0,"irq":0,"soft_irq":0},'
                '"38":{"user":5.2,"nice":0,"system":3.8,"idle":91,"io_wait":0,"irq":0,"soft_irq":0},'
                '"39":{"user":72.69,"nice":0,"system":5.42,"idle":21.89,"io_wait":0,"irq":0,"soft_irq":0},'
                '"40":{"user":75.85,"nice":0,"system":2.4,"idle":21.76,"io_wait":0,"irq":0,"soft_irq":0},'
                '"41":{"user":1,"nice":0,"system":3.6,"idle":95.4,"io_wait":0,"irq":0,"soft_irq":0},'
                '"42":{"user":4.58,"nice":0,"system":3.78,"idle":91.63,"io_wait":0,"irq":0,"soft_irq":0},'
                '"43":{"user":7.62,"nice":0,"system":5.21,"idle":87.17,"io_wait":0,"irq":0,"soft_irq":0},'
                '"44":{"user":6.22,"nice":0,"system":2.81,"idle":90.96,"io_wait":0,"irq":0,"soft_irq":0},'
                '"45":{"user":1.81,"nice":0,"system":4.62,"idle":93.57,"io_wait":0,"irq":0,"soft_irq":0},'
                '"46":{"user":1.8,"nice":0,"system":6.21,"idle":91.98,"io_wait":0,"irq":0,"soft_irq":0},'
                '"47":{"user":1.6,"nice":0,"system":5,"idle":93.4,"io_wait":0,"irq":0,"soft_irq":0},"48":{"user":0,'
                '"nice":0,"system":0.2,"idle":99.8,"io_wait":0,"irq":0,"soft_irq":0},"49":{"user":0.2,"nice":0,'
                '"system":0.2,"idle":99.6,"io_wait":0,"irq":0,"soft_irq":0},"50":{"user":0,"nice":0,"system":1.4,'
                '"idle":98.6,"io_wait":0,"irq":0,"soft_irq":0},"51":{"user":76.15,"nice":0,"system":2.81,'
                '"idle":21.04,"io_wait":0,"irq":0,"soft_irq":0},"52":{"user":8.2,"nice":0,"system":3.4,"idle":88.4,'
                '"io_wait":0,"irq":0,"soft_irq":0},"53":{"user":8.62,"nice":0,"system":3.61,"idle":87.78,"io_wait":0,'
                '"irq":0,"soft_irq":0},"54":{"user":7.62,"nice":0,"system":1,"idle":91.38,"io_wait":0,"irq":0,'
                '"soft_irq":0},"55":{"user":75.3,"nice":0,"system":0.6,"idle":24.1,"io_wait":0,"irq":0,"soft_irq":0},'
                '"56":{"user":0,"nice":0,"system":0,"idle":100,"io_wait":0,"irq":0,"soft_irq":0},"57":{"user":0,'
                '"nice":0,"system":0,"idle":100,"io_wait":0,"irq":0,"soft_irq":0},"58":{"user":0,"nice":0,"system":0,'
                '"idle":100,"io_wait":0,"irq":0,"soft_irq":0},"59":{"user":75.2,"nice":0,"system":0.6,"idle":24.2,'
                '"io_wait":0,"irq":0,"soft_irq":0},"60":{"user":70.46,"nice":0,"system":0,"idle":29.54,"io_wait":0,'
                '"irq":0,"soft_irq":0},"61":{"user":70.34,"nice":0,"system":0,"idle":29.66,"io_wait":0,"irq":0,'
                '"soft_irq":0},"62":{"user":72.8,"nice":0,"system":0,"idle":27.2,"io_wait":0,"irq":0,"soft_irq":0},'
                '"63":{"user":73.2,"nice":0,"system":3,"idle":23.8,"io_wait":0,"irq":0,"soft_irq":0},'
                '"64":{"user":19.8,"nice":0,"system":1,"idle":79.2,"io_wait":0,"irq":0,"soft_irq":0},'
                '"65":{"user":0.8,"nice":0,"system":0,"idle":99.2,"io_wait":0,"irq":0,"soft_irq":0},"66":{"user":0.2,'
                '"nice":0,"system":0,"idle":99.8,"io_wait":0,"irq":0,"soft_irq":0},"67":{"user":0.2,"nice":0,'
                '"system":0,"idle":99.8,"io_wait":0,"irq":0,"soft_irq":0},"68":{"user":6.24,"nice":0,"system":1.61,'
                '"idle":92.15,"io_wait":0,"irq":0,"soft_irq":0},"69":{"user":0.2,"nice":0,"system":0,"idle":99.8,'
                '"io_wait":0,"irq":0,"soft_irq":0},"70":{"user":0.6,"nice":0,"system":2.59,"idle":96.81,"io_wait":0,'
                '"irq":0,"soft_irq":0},"71":{"user":2.79,"nice":0,"system":5.38,"idle":91.83,"io_wait":0,"irq":0,'
                '"soft_irq":0},"72":{"user":1.6,"nice":0,"system":6.01,"idle":92.38,"io_wait":0,"irq":0,'
                '"soft_irq":0},"73":{"user":0.2,"nice":0,"system":0.2,"idle":99.6,"io_wait":0,"irq":0,"soft_irq":0},'
                '"74":{"user":0.2,"nice":0,"system":0,"idle":99.8,"io_wait":0,"irq":0,"soft_irq":0},"75":{"user":0.2,'
                '"nice":0,"system":0,"idle":99.8,"io_wait":0,"irq":0,"soft_irq":0},"76":{"user":0.2,"nice":0,'
                '"system":0.2,"idle":99.6,"io_wait":0,"irq":0,"soft_irq":0},"77":{"user":2.2,"nice":0,"system":5.4,'
                '"idle":92.4,"io_wait":0,"irq":0,"soft_irq":0},"78":{"user":1.4,"nice":0,"system":5.01,"idle":93.59,'
                '"io_wait":0,"irq":0,"soft_irq":0},"79":{"user":0.4,"nice":0,"system":0,"idle":99.6,"io_wait":0,'
                '"irq":0,"soft_irq":0},"80":{"user":0,"nice":0,"system":0.4,"idle":99.6,"io_wait":0,"irq":0,'
                '"soft_irq":0},"81":{"user":2,"nice":0,"system":5.59,"idle":92.42,"io_wait":0,"irq":0,"soft_irq":0},'
                '"82":{"user":2.6,"nice":0,"system":6.4,"idle":91,"io_wait":0,"irq":0,"soft_irq":0},'
                '"83":{"user":2.79,"nice":0,"system":6.37,"idle":90.84,"io_wait":0,"irq":0,"soft_irq":0},'
                '"84":{"user":2.2,"nice":0,"system":5.2,"idle":92.6,"io_wait":0,"irq":0,"soft_irq":0},'
                '"85":{"user":0.2,"nice":0,"system":1.4,"idle":98.4,"io_wait":0,"irq":0,"soft_irq":0},'
                '"86":{"user":2.4,"nice":0,"system":5.21,"idle":92.38,"io_wait":0,"irq":0,"soft_irq":0},'
                '"87":{"user":2.4,"nice":0,"system":6.61,"idle":90.98,"io_wait":0,"irq":0,"soft_irq":0},'
                '"88":{"user":2.2,"nice":0,"system":6.61,"idle":91.18,"io_wait":0,"irq":0,"soft_irq":0},'
                '"89":{"user":0,"nice":0,"system":0.4,"idle":99.6,"io_wait":0,"irq":0,"soft_irq":0},"90":{"user":2.4,'
                '"nice":0,"system":5,"idle":92.6,"io_wait":0,"irq":0,"soft_irq":0},"91":{"user":0.2,"nice":0,'
                '"system":0,"idle":99.8,"io_wait":0,"irq":0,"soft_irq":0},"92":{"user":0,"nice":0,"system":0.2,'
                '"idle":99.8,"io_wait":0,"irq":0,"soft_irq":0},"93":{"user":0,"nice":0,"system":0,"idle":100,'
                '"io_wait":0,"irq":0,"soft_irq":0},"94":{"user":0,"nice":0,"system":0.2,"idle":99.8,"io_wait":0,'
                '"irq":0,"soft_irq":0},"95":{"user":2,"nice":0,"system":5.19,"idle":92.81,"io_wait":0,"irq":0,'
                '"soft_irq":0},"96":{"user":76.2,"nice":0,"system":6.8,"idle":17,"io_wait":0,"irq":0,"soft_irq":0},'
                '"97":{"user":75.15,"nice":0,"system":1.6,"idle":23.25,"io_wait":0,"irq":0,"soft_irq":0},'
                '"98":{"user":1.4,"nice":0,"system":4.81,"idle":93.79,"io_wait":0,"irq":0,"soft_irq":0},'
                '"99":{"user":1.39,"nice":0,"system":4.78,"idle":93.82,"io_wait":0,"irq":0,"soft_irq":0},'
                '"100":{"user":2.19,"nice":0,"system":5.38,"idle":92.43,"io_wait":0,"irq":0,"soft_irq":0},'
                '"101":{"user":75.05,"nice":0,"system":0.6,"idle":24.35,"io_wait":0,"irq":0,"soft_irq":0},'
                '"102":{"user":77.4,"nice":0,"system":3.8,"idle":18.8,"io_wait":0,"irq":0,"soft_irq":0},'
                '"103":{"user":0,"nice":0,"system":0,"idle":100,"io_wait":0,"irq":0,"soft_irq":0},"104":{"user":0,'
                '"nice":0,"system":0,"idle":100,"io_wait":0,"irq":0,"soft_irq":0},"105":{"user":74.95,"nice":0,'
                '"system":1.2,"idle":23.85,"io_wait":0,"irq":0,"soft_irq":0},"106":{"user":76.8,"nice":0,'
                '"system":0.4,"idle":22.8,"io_wait":0,"irq":0,"soft_irq":0},"107":{"user":76.95,"nice":0,'
                '"system":1.2,"idle":21.84,"io_wait":0,"irq":0,"soft_irq":0},"108":{"user":78.71,"nice":0,'
                '"system":7.43,"idle":13.86,"io_wait":0,"irq":0,"soft_irq":0},"109":{"user":75.05,"nice":0,'
                '"system":0.4,"idle":24.55,"io_wait":0,"irq":0,"soft_irq":0},"110":{"user":75.15,"nice":0,'
                '"system":0.4,"idle":24.45,"io_wait":0,"irq":0,"soft_irq":0},"111":{"user":75.15,"nice":0,'
                '"system":0.6,"idle":24.25,"io_wait":0,"irq":0,"soft_irq":0},"112":{"user":75.15,"nice":0,'
                '"system":0.6,"idle":24.25,"io_wait":0,"irq":0,"soft_irq":0},"113":{"user":74.85,"nice":0,'
                '"system":1.2,"idle":23.95,"io_wait":0,"irq":0,"soft_irq":0},"114":{"user":74.85,"nice":0,"system":1,'
                '"idle":24.15,"io_wait":0,"irq":0,"soft_irq":0},"115":{"user":0,"nice":0,"system":0,"idle":100,'
                '"io_wait":0,"irq":0,"soft_irq":0},"116":{"user":77.84,"nice":0,"system":0.8,"idle":21.36,'
                '"io_wait":0,"irq":0,"soft_irq":0},"117":{"user":78.2,"nice":0,"system":0.4,"idle":21.4,"io_wait":0,'
                '"irq":0,"soft_irq":0},"118":{"user":77.8,"nice":0,"system":1,"idle":21.2,"io_wait":0,"irq":0,'
                '"soft_irq":0},"119":{"user":0.4,"nice":0,"system":0,"idle":99.6,"io_wait":0,"irq":0,"soft_irq":0},'
                '"120":{"user":75.2,"nice":0,"system":0.4,"idle":24.4,"io_wait":0,"irq":0,"soft_irq":0},'
                '"121":{"user":75.15,"nice":0,"system":0.6,"idle":24.25,"io_wait":0,"irq":0,"soft_irq":0},'
                '"122":{"user":74.8,"nice":0,"system":1,"idle":24,"io_wait":0,"irq":0,"soft_irq":0.2},'
                '"123":{"user":0.2,"nice":0,"system":0,"idle":99.8,"io_wait":0,"irq":0,"soft_irq":0},"124":{"user":0,'
                '"nice":0,"system":0.2,"idle":99.8,"io_wait":0,"irq":0,"soft_irq":0},"125":{"user":0.2,"nice":0,'
                '"system":0,"idle":99.8,"io_wait":0,"irq":0,"soft_irq":0},"126":{"user":0,"nice":0,"system":0.2,'
                '"idle":99.8,"io_wait":0,"irq":0,"soft_irq":0},"127":{"user":1.2,"nice":0,"system":3,"idle":95.8,'
                '"io_wait":0,"irq":0,"soft_irq":0}},"context_switch_count":171386,"error":""},"neuron_hw_counters":{'
                '"period":1.000142057,"neuron_devices":[{"neuron_device_index":0,"mem_ecc_corrected":1,'
                '"mem_ecc_uncorrected":0,"sram_ecc_uncorrected":0,"sram_ecc_corrected":0},{"neuron_device_index":1,'
                '"mem_ecc_corrected":0,"mem_ecc_uncorrected":0,"sram_ecc_uncorrected":0,"sram_ecc_corrected":0},'
                '{"neuron_device_index":2,"mem_ecc_corrected":1,"mem_ecc_uncorrected":0,"sram_ecc_uncorrected":0,'
                '"sram_ecc_corrected":0},{"neuron_device_index":3,"mem_ecc_corrected":2,"mem_ecc_uncorrected":0,'
                '"sram_ecc_uncorrected":0,"sram_ecc_corrected":0},{"neuron_device_index":4,"mem_ecc_corrected":0,'
                '"mem_ecc_uncorrected":0,"sram_ecc_uncorrected":0,"sram_ecc_corrected":0},{"neuron_device_index":5,'
                '"mem_ecc_corrected":1,"mem_ecc_uncorrected":0,"sram_ecc_uncorrected":0,"sram_ecc_corrected":0},'
                '{"neuron_device_index":6,"mem_ecc_corrected":0,"mem_ecc_uncorrected":1,"sram_ecc_uncorrected":0,'
                '"sram_ecc_corrected":0},{"neuron_device_index":7,"mem_ecc_corrected":0,"mem_ecc_uncorrected":0,'
                '"sram_ecc_uncorrected":1,"sram_ecc_corrected":0},{"neuron_device_index":8,"mem_ecc_corrected":0,'
                '"mem_ecc_uncorrected":0,"sram_ecc_uncorrected":0,"sram_ecc_corrected":1},{"neuron_device_index":9,'
                '"mem_ecc_corrected":0,"mem_ecc_uncorrected":1,"sram_ecc_uncorrected":0,"sram_ecc_corrected":0},'
                '{"neuron_device_index":10,"mem_ecc_corrected":0,"mem_ecc_uncorrected":0,"sram_ecc_uncorrected":0,'
                '"sram_ecc_corrected":0},{"neuron_device_index":11,"mem_ecc_corrected":0,"mem_ecc_uncorrected":0,'
                '"sram_ecc_uncorrected":0,"sram_ecc_corrected":0},{"neuron_device_index":12,"mem_ecc_corrected":0,'
                '"mem_ecc_uncorrected":0,"sram_ecc_uncorrected":1,"sram_ecc_corrected":0},{"neuron_device_index":13,'
                '"mem_ecc_corrected":0,"mem_ecc_uncorrected":0,"sram_ecc_uncorrected":0,"sram_ecc_corrected":1},'
                '{"neuron_device_index":14,"mem_ecc_corrected":0,"mem_ecc_uncorrected":1,"sram_ecc_uncorrected":1,'
                '"sram_ecc_corrected":0},{"neuron_device_index":15,"mem_ecc_corrected":0,"mem_ecc_uncorrected":1,'
                '"sram_ecc_uncorrected":0,"sram_ecc_corrected":0}],"error":""}},"instance_info":{'
                '"instance_name":"DummyNodeName",'
                '"instance_id":"i-09db9b55e0095612f","instance_type":"trn1n.32xlarge",'
                '"instance_availability_zone":"us-east-1c","instance_availability_zone_id":"use1-az6",'
                '"instance_region":"us-east-1","ami_id":"ami-030686a4e905e98d3",'
                '"subnet_id":"subnet-06a7754948e8a000f","error":""},"neuron_hardware_info":{"neuron_device_count":16,'
                '"neuroncore_per_device_count":2,"error":""}}')
        if len(line) == 0:
            continue
        if original_file_hash:
            _watch_file_and_update_ssl_cxt(original_file_hash, certfile=certfile, keyfile=keyfile)
        try:
            monitor_data = json.loads(line)
        except Exception as exc:
            print('Unable to decode JSON {}'.format(exc))
            continue
        if instance_labels is None:
            instance_labels = get_instance_labels(monitor_data['instance_info'])
        process_data(all_metric_objects, monitor_data, instance_labels)
        time.sleep(5)

def main():
    global ssl_cxt
    arg_parser = argparse.ArgumentParser()
    arg_parser.add_argument('-p', '--port', default=8000,
                            type=int, help='HTTP port on which to run the server')
    arg_parser.add_argument('--key-file', help='Path to SSL private key file (only for HTTPS)')
    arg_parser.add_argument('--cert-file', help='Path to SSL certificate file (only for HTTPS)')
    args = arg_parser.parse_args()

    if args.key_file and args.cert_file:
        if sys.version_info < (3, 8):
            print("""Python version 3.8 or greater is requried for https/tls support.
                    Also upgrade your prometheus_client version to 0.19.0 or greater if required
                    https://github.com/prometheus/client_python/releases""")
            sys.exit(1)
        httpd, _t = start_http_server(port=args.port, keyfile=args.key_file, certfile=args.cert_file)
        ssl_cxt = httpd.socket.context
        print("Running HTTPS prometheus server at port {}".format(args.port))
    else:
        start_http_server(port=args.port)
        print("Running HTTP prometheus server at port {}".format(args.port))

    update_loop(certfile = args.cert_file or None, keyfile=args.key_file or None)


if __name__ == '__main__':
    main()
