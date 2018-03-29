FROM        quay.io/prometheus/busybox:latest

COPY bin/azure_metrics_exporter /bin/azure_metrics_exporter

EXPOSE      9276
ENTRYPOINT  [ "/bin/azure_metrics_exporter" ]
