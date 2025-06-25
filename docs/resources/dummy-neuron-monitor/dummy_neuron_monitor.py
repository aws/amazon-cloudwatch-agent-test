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
        if original_file_hash:
            _watch_file_and_update_ssl_cxt(original_file_hash, certfile=certfile, keyfile=keyfile)
        try:
            monitor_data = {}
            with open('/opt/aws/neuron/bin/neuron-monitor-output.json', 'r') as file:
                json_data = json.load(file)
            monitor_data = json_data["1"]
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
