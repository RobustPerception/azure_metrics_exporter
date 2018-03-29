FROM quay.io/prometheus/busybox:glibc
COPY bin/azure_metrics_exporter /azure_metrics_exporter
EXPOSE      9276
ENTRYPOINT  [ "/azure_metrics_exporter" ]
