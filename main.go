package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/conorbro/azure-metrics-exporter/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/version"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	sc = &config.SafeConfig{
		C: &config.Config{},
	}
	ac            = NewAzureClient()
	configFile    = kingpin.Flag("config.file", "Azure exporter configuration file.").Default("azure.yml").String()
	listenAddress = kingpin.Flag("web.listen-address", "The address to listen on for HTTP requests.").Default(":9276").String()
)

// AzureMetricDefinitionResponse represents metric definition response for a given resource from Azure.
type AzureMetricDefinitionResponse struct {
	MetricDefinitionResponses []metricDefinitionResponse `json:"value"`
}
type metricDefinitionResponse struct {
	Dimensions             []dimensionData      `json:"dimensions"`
	ID                     string               `json:"id"`
	IsDimensionRequired    bool                 `json:"isDimensionRequired"`
	MetricAvailabilities   []metricAvailability `json:"metricAvailabilities"`
	Name                   metricData           `json:"name"`
	PrimaryAggregationType string               `json:"primaryAggregationType"`
	ResourceID             string               `json:"resourceId"`
	Unit                   string               `json:"unit"`
}
type dimensionData struct {
	LocalizedValue string `json:"localizedValue"`
	Value          string `json:"value"`
}
type metricAvailability struct {
	Retention string `json:"retention"`
	TimeGrain string `json:"timeGrain"`
}
type metricData struct {
	LocalizedValue string `json:"localizedValue"`
	Value          string `json:"value"`
}

// AzureMetricValiueResponse represents metric value response for a given metric definition.
type AzureMetricValiueResponse struct {
	Value []data `json:"value"`
}
type data struct {
	Data []metricDataPoint `json:"data"`
	ID   string            `json:"id"`
	Name dimensionData     `json:"name"`
	Type string            `json:"type"`
	Unit string            `json:"unit"`
}
type metricDataPoint struct {
	TimeStamp string  `json:"timeStamp"`
	Total     float64 `json:"total"`
}

func init() {
	prometheus.MustRegister(version.NewCollector("azure_exporter"))
}

func (ac *AzureClient) getAccessToken() {
	target := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/token", sc.C.Credentials.TenantID)
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"resource":      {"https://management.azure.com/"},
		"client_id":     {sc.C.Credentials.ClientID},
		"client_secret": {sc.C.Credentials.ClientSecret},
	}
	resp, err := ac.client.PostForm(target, form)
	if err != nil {
		log.Fatalf("Error authenticating against Azure API: %v", err)
	}
	if resp.StatusCode != 200 {
		log.Fatalf("Did not get status code 200, got: %d", resp.StatusCode)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Error reading body of response: %v", err)
	}

	var data map[string]interface{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		log.Fatalf("Error unmarshalling response body: %v", err)
	}
	ac.accessToken = data["access_token"].(string)
}

func (ac *AzureClient) getMetricDefinitions() AzureMetricDefinitionResponse {
	metricsResource := fmt.Sprintf("subscriptions/%s%s", sc.C.Credentials.SubscriptionID, sc.C.TargetResource)
	apiVersion := "2016-03-01"
	metricsTarget := fmt.Sprintf("https://management.azure.com/%s/providers/microsoft.insights/metricDefinitions?api-version=%s", metricsResource, apiVersion)

	req, err := http.NewRequest("GET", metricsTarget, nil)
	if err != nil {
		log.Fatalf("Error creating HTTP request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+ac.accessToken)
	resp, err := ac.client.Do(req)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Error reading body of response: %v", err)
	}

	var data AzureMetricDefinitionResponse
	err = json.Unmarshal(body, &data)
	if err != nil {
		log.Fatalf("Error unmarshalling response body: %v", err)
	}

	return data
}

func (ac *AzureClient) getMetricValue(metricName string) AzureMetricValiueResponse {
	metricsResource := fmt.Sprintf("subscriptions/%s%s", sc.C.Credentials.SubscriptionID, sc.C.TargetResource)
	apiVersion := "2016-09-01"
	filter := fmt.Sprintf("(name.value eq '%s') and aggregationType eq 'Total' and startTime eq 2017-11-01T17:05 and endTime eq 2017-11-01T17:15", metricName)
	filter = strings.Replace(filter, " ", "%20", -1)
	metricValueEndpoint := fmt.Sprintf("https://management.azure.com/%s/providers/microsoft.insights/metrics?$filter=%s&api-version=%s", metricsResource, filter, apiVersion)

	req, err := http.NewRequest("GET", metricValueEndpoint, nil)
	if err != nil {
		log.Fatalf("Error creating HTTP request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+ac.accessToken)
	resp, err := ac.client.Do(req)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Error reading body of response: %v", err)
	}

	var data AzureMetricValiueResponse
	err = json.Unmarshal(body, &data)
	if err != nil {
		log.Fatalf("Error unmarshalling response body: %v", err)
	}
	return data
}

// Collector type contains target resource and metrics we wish to query for.
type Collector struct {
	target  string
	metrics []config.Metric
}

// Describe implemented with dummy data to satisfy interface.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- prometheus.NewDesc("dummy", "dummy", nil, nil)
}

// Collect ...
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	// do get Azure metric result from API and create Prometheus metric from it
	// Get metric values for all defined metrics
	var metricValueData AzureMetricValiueResponse
	for _, m := range sc.C.Metrics {
		metricValueData = ac.getMetricValue(m.Name)
	}

	for _, m := range metricValueData.Value[0].Data {
		fmt.Println(m.TimeStamp, m.Total)
	}
	fmt.Println(metricValueData.Value[0].Name.Value, ", Num samples:", len(metricValueData.Value[0].Data))

	metricName := metricValueData.Value[0].Name.Value
	metricValue := metricValueData.Value[0].Data[len(metricValueData.Value[0].Data)-1]

	// Determine metric type from metric aggregation type

	// Create metric based on result from Azure monitor API
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(metricName, "help?", nil, nil),
		prometheus.GaugeValue,
		metricValue.Total,
	)

}

// NewCollector ...
func NewCollector(target string, metrics []config.Metric) *Collector {
	return &Collector{
		target:  target,
		metrics: metrics,
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	registry := prometheus.NewRegistry()
	collector := NewCollector(sc.C.TargetResource, sc.C.Metrics)
	registry.MustRegister(collector)
	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
}

// AzureClient represents our client to talk to the Azure api
type AzureClient struct {
	client      *http.Client
	accessToken string
}

// NewAzureClient returns an Azure client to talk the Azure API
func NewAzureClient() *AzureClient {
	return &AzureClient{
		client:      &http.Client{},
		accessToken: "",
	}
}

// CpuTime Total
// Requests Total
// BytesReceived Total
// BytesSent Total
// Http101 Total
// Http2xx Total
// Http3xx Total
// Http401 Total
// Http403 Total
// Http404 Total
// Http406 Total
// Http4xx Total
// Http5xx Total
// MemoryWorkingSet Average
// AverageMemoryWorkingSet Average
// AverageResponseTime Average

func main() {
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	if err := sc.ReloadConfig(*configFile); err != nil {
		log.Fatalf("Error loading config: %v", err)
		os.Exit(1)
	}

	ac.getAccessToken()

	// Get all metric definitions if no metric names in config
	// metricDefinitionData := ac.getMetricDefinitions()
	// for _, metricDefinition := range metricDefinitionData.MetricDefinitionResponses {
	// 	fmt.Println(metricDefinition.Name.Value, metricDefinition.PrimaryAggregationType)
	// sc.C.Metrics = append(sc.C.Metrics, config.Metric{Name: metricDefinition.Name.Value})
	// }

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
	// http.Handle("/metrics", promhttp.Handler())
	if err := http.ListenAndServe(*listenAddress, nil); err != nil {
		log.Fatalf("Error starting HTTP server: %v", err)
		os.Exit(1)
	}

}
