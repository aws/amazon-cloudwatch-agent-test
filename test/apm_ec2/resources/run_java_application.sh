#!/bin/sh

JAVA_TOOL_OPTIONS=" -javaagent:${HOME}/aws-apm-0.3.0.jar" \
OTEL_METRICS_EXPORTER=none \
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4315 \
OTEL_TRACES_SAMPLER_ARG="1.0" \
OTEL_RESOURCE_ATTRIBUTES=aws.hostedin.environment=alpha/pet-clinic,service.name=pet-clinic-frontend \
java -Dspring.cloud.discovery.enabled=false -jar "${HOME}"/spring-petclinic-api-gateway-2.6.7.jar > /tmp/log 2>&1