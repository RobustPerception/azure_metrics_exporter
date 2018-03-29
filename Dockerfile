FROM debian:9-slim
RUN apt-get update && apt-get install -yq --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
COPY bin/azure_metrics_exporter /azure_metrics_exporter
EXPOSE      9276
ENTRYPOINT  [ "/azure_metrics_exporter" ]
