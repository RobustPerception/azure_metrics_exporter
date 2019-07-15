#!/bin/bash
make build

### PHASE 1
echo 'copying base config'
cp base_conf.yml azure.yml

echo "starting exporter"
./azure_metrics_exporter &
pid=$!

echo "giving time for exporter startup"
sleep 5
echo "exporter started: $pid"

for i in {1..5}
do
    echo "running scrape #$i"
    curl -XGET localhost:9276/metrics
    sleep 10
done

echo "base config tests done. stopping exporter"
kill -9 $pid

### PHASE 2
echo 'copying extended config'
cp extended_conf.yml azure.yml

echo "starting exporter"
./azure_metrics_exporter &
pid=$!

echo "giving time for exporter startup"
sleep 5
echo "exporter started: $pid"

for i in {1..5}
do
    echo "running scrape #$i"
    curl -XGET localhost:9276/metrics
    sleep 10
done

echo "tests done. stopping exporter"
kill -9 $pid