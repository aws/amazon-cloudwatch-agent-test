# set the base image
FROM public.ecr.aws/docker/library/ubuntu:20.04

# Set the working directory in the container
WORKDIR /root

# Neuron SDK components version numbers
ARG NEURONX_RUNTIME_LIB_VERSION=2.19.*
ARG NEURONX_COLLECTIVES_LIB_VERSION=2.19.*
ARG NEURONX_TOOLS_VERSION=2.17.*

# Install any necessary dependencies or tools
RUN apt-get update && apt-get install vim wget zip unzip sudo python3-pip -y

# Import Neuron GPG public key
RUN echo "deb https://apt.repos.neuron.amazonaws.com focal main" > /etc/apt/sources.list.d/neuron.list
RUN wget -qO - https://apt.repos.neuron.amazonaws.com/GPG-PUB-KEY-AMAZON-AWS-NEURON.PUB | apt-key add -


# Install Neuron Runtime and Neuron Tools
RUN apt-get update \
 && apt-get install -y \
    aws-neuronx-tools=$NEURONX_TOOLS_VERSION \
    aws-neuronx-collectives=$NEURONX_COLLECTIVES_LIB_VERSION \
    aws-neuronx-runtime-lib=$NEURONX_RUNTIME_LIB_VERSION \
 && rm -rf /var/lib/apt/lists/* \
 && rm -rf /tmp/tmp* \
 && apt-get clean

COPY neuron-monitor-output.json /opt/aws/neuron/bin/neuron-monitor-output.json
COPY dummy_neuron_monitor.py /opt/aws/neuron/bin/dummy_neuron_monitor.py
RUN chmod 755 /opt/aws/neuron/bin/dummy_neuron_monitor.py
RUN pip3 install prometheus_client boto3 requests