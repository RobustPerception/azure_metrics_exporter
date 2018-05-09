FROM golang:1.10.1-alpine AS build

COPY . $GOPATH/src/github.com/RobustPerception/azure_metrics_exporter

RUN apk --update add git make
RUN cd $GOPATH/src/github.com/RobustPerception/azure_metrics_exporter \
  && make build && cp azure-metrics-exporter /bin/azure-metrics-exporter

FROM quay.io/prometheus/busybox:latest

COPY --from=build /bin/azure-metrics-exporter /bin/azure-metrics-exporter

EXPOSE 9276
ENTRYPOINT ["/bin/azure-metrics-exporter"]
