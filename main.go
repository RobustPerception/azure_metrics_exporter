package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"

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

	ac = NewAzureClient()

	configFile    = kingpin.Flag("config.file", "Azure exporter configuration file.").Default("azure.yml").String()
	listenAddress = kingpin.Flag("web.listen-address", "The address to listen on for HTTP requests.").Default(":9276").String()

	apiVersion = "2017-05-01-preview"
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
	Cost         float64 `json:"cost"`
	Interval     string  `json:"interval"`
	Timespan     string  `json:"timespan"`
	Value        []data  `json:"value"`
	ResponstType string  `json:"type"`
	Unit         string  `json:"unit"`
}
type data struct {
	ID         string           `json:"id"`
	Name       dimensionData    `json:"name"`
	TimeSeries []timeSeriesData `json:"timeseries"`
}
type timeSeriesData struct {
	TimeSeriesData []timeSeriesDataPoints `json:"data"`
	Metadatavalues []string               `json:"metadatavalues"`
}
type timeSeriesDataPoints struct {
	Average   float64 `json:"average"`
	Timestamp string  `json:"timestamp"`
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
	metricValueEndpoint := fmt.Sprintf("https://management.azure.com/%s/providers/microsoft.insights/metrics?metric=%s&aggregation=Total&api-version=%s", metricsResource, metricName, apiVersion)

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
	for _, m := range sc.C.Metrics {
		metricValueData := ac.getMetricValue(m.Name)
		fmt.Printf("%+v", metricValueData)
	}

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

func main() {
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	if err := sc.ReloadConfig(*configFile); err != nil {
		log.Fatalf("Error loading config: %v", err)
		os.Exit(1)
	}

	ac.getAccessToken()

	// Get all metric definitions if no metric names in config
	// if len(sc.C.Metrics) == 0 {
	// metricDefinitionData := ac.getMetricDefinitions()
	// for _, metricDefinition := range metricDefinitionData.MetricDefinitionResponses {
	// 	fmt.Println(metricDefinition.Name.Value, metricDefinition.PrimaryAggregationType)
	// sc.C.Metrics = append(sc.C.Metrics, config.Metric{Name: metricDefinition.Name.Value})
	// }
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
		// level.Error(logger).Log("msg", "Error starting HTTP server")
		os.Exit(1)
	}

}
