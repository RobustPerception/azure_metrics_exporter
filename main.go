package main

import (
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/RobustPerception/azure_metrics_exporter/config"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/version"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	sc = &config.SafeConfig{
		C: &config.Config{},
	}
	ac                    = NewAzureClient()
	configFile            = kingpin.Flag("config.file", "Azure exporter configuration file.").Default("azure.yml").String()
	listenAddress         = kingpin.Flag("web.listen-address", "The address to listen on for HTTP requests.").Default(":9276").String()
	listMetricDefinitions = kingpin.Flag("list.definitions", "List available metric definitions for the given resources and exit.").Bool()
	invalidMetricChars    = regexp.MustCompile("[^a-zA-Z0-9_:]")
	azureErrorDesc        = prometheus.NewDesc("azure_error", "Error collecting metrics", nil, nil)
	batchSize             = 20
	publishedResource     = map[string]bool{}
	resourceList          map[string]AzureResource
)

func init() {
	prometheus.MustRegister(version.NewCollector("azure_exporter"))
}

// Collector generic collector type
type Collector struct{}

// Describe implemented with dummy data to satisfy interface.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- prometheus.NewDesc("dummy", "dummy", nil, nil)
}

type resourceMeta struct {
	resourceURL  string
	metrics      string
	aggregations []string
	resource     AzureResource
}

func (c *Collector) extractMetrics(ch chan<- prometheus.Metric, rm resourceMeta, httpStatusCode int, metricValueData AzureMetricValueResponse) {
	if httpStatusCode != 200 {
		log.Printf("Received %d status for resource %s. %s", httpStatusCode, rm.resourceURL, metricValueData.APIError.Message)
		return
	}

	if len(metricValueData.Value) == 0 || len(metricValueData.Value[0].Timeseries) == 0 {
		log.Printf("Metric %v not found at target %v\n", rm.metrics, rm.resourceURL)
		return
	}
	if len(metricValueData.Value[0].Timeseries[0].Data) == 0 {
		log.Printf("No metric data returned for metric %v at target %v\n", rm.metrics, rm.resourceURL)
		return
	}

	for _, value := range metricValueData.Value {
		// Ensure Azure metric names conform to Prometheus metric name conventions
		metricName := strings.Replace(value.Name.Value, " ", "_", -1)
		metricName = strings.ToLower(metricName + "_" + value.Unit)
		metricName = strings.Replace(metricName, "/", "_per_", -1)
		metricName = invalidMetricChars.ReplaceAllString(metricName, "_")
		metricValue := value.Timeseries[0].Data[len(value.Timeseries[0].Data)-1]
		labels := CreateResourceLabels(rm.resourceURL)

		if hasAggregation(rm.aggregations, "Total") {
			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc(metricName+"_total", metricName+"_total", nil, labels),
				prometheus.GaugeValue,
				metricValue.Total,
			)
		}

		if hasAggregation(rm.aggregations, "Average") {
			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc(metricName+"_average", metricName+"_average", nil, labels),
				prometheus.GaugeValue,
				metricValue.Average,
			)
		}

		if hasAggregation(rm.aggregations, "Minimum") {
			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc(metricName+"_min", metricName+"_min", nil, labels),
				prometheus.GaugeValue,
				metricValue.Minimum,
			)
		}

		if hasAggregation(rm.aggregations, "Maximum") {
			ch <- prometheus.MustNewConstMetric(
				prometheus.NewDesc(metricName+"_max", metricName+"_max", nil, labels),
				prometheus.GaugeValue,
				metricValue.Maximum,
			)
		}
	}

	if _, ok := publishedResource[rm.resource.ID]; !ok {
		infoLabels := CreateAllResourceLabelsFrom(rm)
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc("azure_resource_info", "Azure information available for resource", nil, infoLabels),
			prometheus.GaugeValue,
			1,
		)
		publishedResource[rm.resource.ID] = true
	}
}

func (c *Collector) batchCollectResources(ch chan<- prometheus.Metric, resources []resourceMeta) {
	// collect metrics in batches
	for i := 0; i < len(resources); i += batchSize {
		j := i + batchSize

		// don't forget to add remainder resources
		if j > len(resources) {
			j = len(resources)
		}

		var urls []string
		for _, r := range resources[i:j] {
			urls = append(urls, r.resourceURL)
		}

		batchData, err := ac.getBatchMetricValues(urls)
		if err != nil {
			ch <- prometheus.NewInvalidMetric(azureErrorDesc, err)
			return
		}

		for k, resp := range batchData.Responses {
			c.extractMetrics(ch, resources[i+k], resp.HttpStatusCode, resp.Content)
		}
	}
}

// Collect - collect results from Azure Montior API and create Prometheus metrics.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	if err := ac.refreshAccessToken(); err != nil {
		log.Println(err)
		ch <- prometheus.NewInvalidMetric(azureErrorDesc, err)
		return
	}

	publishedResource = map[string]bool{}
	resourceList = map[string]AzureResource{}
	var resources []resourceMeta

	for _, target := range sc.C.Targets {
		var rm resourceMeta

		metrics := []string{}
		for _, metric := range target.Metrics {
			metrics = append(metrics, metric.Name)
		}

		rm.metrics = strings.Join(metrics, ",")
		rm.aggregations = filterAggregations(target.Aggregations)
		rm.resourceURL = resourceURLFrom(target.Resource, rm.metrics, rm.aggregations)

		if _, ok := resourceList[target.Resource]; !ok {
			resource, err := ac.lookupResourceByID(target.Resource)
			if err != nil {
				log.Printf("failed to get resource information for target %s: %v", target.Resource, err)
				ch <- prometheus.NewInvalidMetric(azureErrorDesc, err)
				continue
			}
			resourceList[target.Resource] = resource
		}
		rm.resource = resourceList[target.Resource]
		resources = append(resources, rm)
	}

	for _, resourceGroup := range sc.C.ResourceGroups {
		metrics := []string{}
		for _, metric := range resourceGroup.Metrics {
			metrics = append(metrics, metric.Name)
		}
		metricsStr := strings.Join(metrics, ",")

		filteredResources, err := ac.filteredListFromResourceGroup(resourceGroup)
		if err != nil {
			log.Printf("Failed to get resources for resource group %s and resource types %s: %v",
				resourceGroup.ResourceGroup, resourceGroup.ResourceTypes, err)
			ch <- prometheus.NewInvalidMetric(azureErrorDesc, err)
			return
		}

		for _, f := range filteredResources {
			var rm resourceMeta
			rm.metrics = metricsStr
			rm.aggregations = filterAggregations(resourceGroup.Aggregations)
			rm.resourceURL = resourceURLFrom(f.ID, rm.metrics, rm.aggregations)
			rm.resource = f
			resourceList[f.ID] = f
			resources = append(resources, rm)
		}
	}

	for _, resourceTag := range sc.C.ResourceTags {
		metrics := []string{}
		for _, metric := range resourceTag.Metrics {
			metrics = append(metrics, metric.Name)
		}
		metricsStr := strings.Join(metrics, ",")

		filteredResources, err := ac.filteredListByTag(resourceTag)
		if err != nil {
			log.Printf("Failed to get resources for tag name %s, tag value %s: %v",
				resourceTag.ResourceTagName, resourceTag.ResourceTagValue, err)
			ch <- prometheus.NewInvalidMetric(azureErrorDesc, err)
			return
		}

		for _, f := range filteredResources {
			var rm resourceMeta
			rm.metrics = metricsStr
			rm.aggregations = filterAggregations(resourceTag.Aggregations)
			rm.resourceURL = resourceURLFrom(f.ID, rm.metrics, rm.aggregations)

			if _, ok := resourceList[f.ID]; !ok {
				resource, err := ac.lookupResourceByID(f.ID)
				if err != nil {
					log.Printf("failed to get resource information for target %s: %v", f.ID, err)
					ch <- prometheus.NewInvalidMetric(azureErrorDesc, err)
					continue
				}
				resourceList[f.ID] = resource
			}
			rm.resource = resourceList[f.ID]
			resources = append(resources, rm)
		}
	}
	c.batchCollectResources(ch, resources)
}

func handler(w http.ResponseWriter, r *http.Request) {
	registry := prometheus.NewRegistry()
	collector := &Collector{}
	registry.MustRegister(collector)
	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
}

func main() {
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	if err := sc.ReloadConfig(*configFile); err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	err := ac.getAccessToken()
	if err != nil {
		log.Fatalf("Failed to get token: %v", err)
	}

	// Print list of available metric definitions for each resource to console if specified.
	if *listMetricDefinitions {
		results, err := ac.getMetricDefinitions()
		if err != nil {
			log.Fatalf("Failed to fetch metric definitions: %v", err)
		}

		for k, v := range results {
			log.Printf("Resource: %s\n\nAvailable Metrics:\n", strings.Split(k, "/")[6])
			for _, r := range v.MetricDefinitionResponses {
				log.Printf("- %s\n", r.Name.Value)
			}
		}
		os.Exit(0)
	}

	err = ac.listAPIVersions()
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
            <head>
            <title>Azure Exporter</title>
            </head>
            <body>
            <h1>Azure Exporter</h1>
						<p><a href="/metrics">Metrics</a></p>
            </body>
            </html>`))
	})

	http.HandleFunc("/metrics", handler)
	log.Printf("azure_metrics_exporter listening on port %v", *listenAddress)
	if err := http.ListenAndServe(*listenAddress, nil); err != nil {
		log.Fatalf("Error starting HTTP server: %v", err)
	}
}
