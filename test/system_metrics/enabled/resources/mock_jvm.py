#!/usr/bin/env python3
"""Minimal mock JVM agent. Binds @aws-jvm-metrics-<pid> (SOCK_DGRAM), responds to GET /metrics."""
import socket, os
s = socket.socket(socket.AF_UNIX, socket.SOCK_DGRAM)
s.bind(f"\x00aws-jvm-metrics-{os.getpid()}".encode())
R = b"jvm_heap_max_bytes 2147483648\njvm_heap_committed_bytes 1073741824\njvm_heap_after_gc_bytes 536870912\njvm_gc_count_total 42\njvm_allocated_bytes 8589934592\n"
while True:
    d, a = s.recvfrom(1024)
    if d.startswith(b"GET /metrics"):
        s.sendto(R, a)
