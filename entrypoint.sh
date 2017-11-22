#!/bin/bash

/opt/go/bin/azure-metrics-exporter --web.listen-address=":9090" --config.file="/config/${CONFIG_FILE}"
