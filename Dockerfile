FROM alpine

RUN apk update
RUN apk add bash go git musl-dev
RUN mkdir -p /opt/go
ENV GOPATH=/opt/go
RUN go get github.com/carlozleite/azure-metrics-exporter 
ADD entrypoint.sh /entrypoint.sh
RUN chmod 755 /entrypoint.sh
RUN mkdir /config

ENTRYPOINT ["/entrypoint.sh"]

