FROM amazonlinux:2023

RUN yum install -y systemd procps golang sudo tar gzip && yum clean all

# Clean up unnecessary systemd units for container usage
RUN (cd /lib/systemd/system/sysinit.target.wants/ && for i in *; do [ "$i" = "systemd-tmpfiles-setup.service" ] || rm -f "$i"; done) && \
    rm -f /lib/systemd/system/multi-user.target.wants/* && \
    rm -f /etc/systemd/system/*.wants/* && \
    rm -f /lib/systemd/system/local-fs.target.wants/* && \
    rm -f /lib/systemd/system/sockets.target.wants/*udev* && \
    rm -f /lib/systemd/system/sockets.target.wants/*initctl* && \
    rm -f /lib/systemd/system/basic.target.wants/*

COPY .build/amazon-cloudwatch-agent.rpm /tmp/amazon-cloudwatch-agent.rpm
RUN yum install -y /tmp/amazon-cloudwatch-agent.rpm && yum clean all && rm -f /tmp/amazon-cloudwatch-agent.rpm

RUN mkdir -p /etc/systemd/system/amazon-cloudwatch-agent.service.d && \
    printf '[Service]\nPassEnvironment=AWS_EC2_METADATA_SERVICE_ENDPOINT\n' \
    > /etc/systemd/system/amazon-cloudwatch-agent.service.d/override.conf

VOLUME ["/sys/fs/cgroup", "/run", "/tmp"]

STOPSIGNAL SIGRTMIN+3

ENTRYPOINT ["/usr/sbin/init"]
