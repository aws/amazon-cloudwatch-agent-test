countLogLines() {
    log_path="${1:-}"
    if [ -f "${log_path}" ]; then
        cat ${log_path} | wc -l
    else
        echo 0
    fi
}
cwa_log=$(countLogLines "/opt/aws/amazon-cloudwatch-agent/logs/amazon-cloudwatch-agent.log")
echo "cwa_log:${cwa_log}"