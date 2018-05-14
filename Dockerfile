
FROM quay.io/prometheus/busybox:latest

COPY azure-metrics-exporter /bin/azure-metrics-exporter

EXPOSE 9276
ENTRYPOINT ["/bin/azure-metrics-exporter"]

