#!/bin/bash

docker build -t carlozleite/azure-metrics-exporter . --no-cache
docker push carlozleite/azure-metrics-exporter
