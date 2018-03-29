FROM debian:9-slim
COPY bin/azure_metrics_exporter /azure_metrics_exporter
EXPOSE      9276
ENTRYPOINT  [ "/azure_metrics_exporter" ]
