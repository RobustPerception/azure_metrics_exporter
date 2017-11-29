package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
)

// AzureMetricDefinitionResponse represents metric definition response for a given resource from Azure.
type AzureMetricDefinitionResponse struct {
	MetricDefinitionResponses []metricDefinitionResponse `json:"value"`
}
type metricDefinitionResponse struct {
	Dimensions []struct {
		LocalizedValue string `json:"localizedValue"`
		Value          string `json:"value"`
	} `json:"dimensions"`
	ID                   string `json:"id"`
	IsDimensionRequired  bool   `json:"isDimensionRequired"`
	MetricAvailabilities []struct {
		Retention string `json:"retention"`
		TimeGrain string `json:"timeGrain"`
	} `json:"metricAvailabilities"`
	Name struct {
		LocalizedValue string `json:"localizedValue"`
		Value          string `json:"value"`
	} `json:"name"`
	PrimaryAggregationType string `json:"primaryAggregationType"`
	ResourceID             string `json:"resourceId"`
	Unit                   string `json:"unit"`
}

// AzureMetricValueResponse represents a metric value response for a given metric definition.
type AzureMetricValueResponse struct {
	Value []struct {
		Data []struct {
			TimeStamp string  `json:"timeStamp"`
			Total     float64 `json:"total"`
		} `json:"data"`
		ID   string `json:"id"`
		Name struct {
			LocalizedValue string `json:"localizedValue"`
			Value          string `json:"value"`
		} `json:"name"`
		Type string `json:"type"`
		Unit string `json:"unit"`
	} `json:"value"`
	APIError struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
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

// Loop through all specified resource targets and get their respective metric definitions.
func (ac *AzureClient) getMetricDefinitions() map[string]AzureMetricDefinitionResponse {
	apiVersion := "2016-03-01"
	definitions := make(map[string]AzureMetricDefinitionResponse)

	for _, target := range sc.C.Targets {
		metricsResource := fmt.Sprintf("subscriptions/%s%s", sc.C.Credentials.SubscriptionID, target.Resource)
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

		def := AzureMetricDefinitionResponse{}
		err = json.Unmarshal(body, &def)
		if err != nil {
			log.Fatalf("Error unmarshalling response body: %v", err)
		}
		definitions[target.Resource] = def
	}
	return definitions
}

func (ac *AzureClient) getMetricValue(metricName string, target string) AzureMetricValueResponse {
	apiVersion := "2016-09-01"
	metricsResource := fmt.Sprintf("subscriptions/%s%s", sc.C.Credentials.SubscriptionID, target)
	now, oneMinutesAgo := GetTimes()
	filter := fmt.Sprintf("(name.value eq '%s') and aggregationType eq 'Total' and startTime eq %s and endTime eq %s", metricName, oneMinutesAgo, now)
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

	var data AzureMetricValueResponse
	err = json.Unmarshal(body, &data)
	if err != nil {
		log.Fatalf("Error unmarshalling response body: %v", err)
	}
	if data.APIError.Code == "ExpiredAuthenticationToken" {
		log.Printf("Access token expired. Reathenticating...")
		ac.getAccessToken()
	}
	return data
}
