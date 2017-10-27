package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
)

// Credentials struct for storing Azure credentials
type Credentials struct {
	SubscriptionID string `json:"subscription_id"`
	ClientID       string `json:"client_id"`
	ClientSecret   string `json:"client_secret"`
	TenantID       string `json:"tenant_id"`
}

// AzureMetricDefinitionResponse represents metric definition response for a given resource from Azure
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

func createCredentials() Credentials {
	path := "/Users/conorbroderick/.azure/credentials.json"
	file, err := ioutil.ReadFile(path)
	if err != nil {
		log.Fatalf("Unable to create credentials from file %s: %s", path, err)
	}
	var c Credentials
	json.Unmarshal(file, &c)
	return c
}

func getAccessToken(client *http.Client, creds Credentials) string {
	target := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/token", creds.TenantID)
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"resource":      {"https://management.azure.com/"},
		"client_id":     {creds.ClientID},
		"client_secret": {creds.ClientSecret},
	}
	resp, err := client.PostForm(target, form)
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
	return data["access_token"].(string)
}

func getMetricDefinitions(client *http.Client, creds Credentials, accessToken string) AzureMetricDefinitionResponse {
	metricsResource := fmt.Sprintf("subscriptions/%s/resourceGroups/blog-aisling/providers/Microsoft.Web/sites/blog-aisling", creds.SubscriptionID)
	metricsTarget := fmt.Sprintf("https://management.azure.com/%s/providers/microsoft.insights/metricDefinitions?api-version=2017-05-01-preview", metricsResource)
	req, err := http.NewRequest("GET", metricsTarget, nil)
	if err != nil {
		log.Fatalf("Error creating HTTP request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := client.Do(req)
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

func main() {
	creds := createCredentials()
	client := &http.Client{}
	accessToken := getAccessToken(client, creds)
	metridDefinitionData := getMetricDefinitions(client, creds, accessToken)

	for _, metricDefinition := range metridDefinitionData.MetricDefinitionResponses {
		fmt.Println(metricDefinition.Name)
	}

}
