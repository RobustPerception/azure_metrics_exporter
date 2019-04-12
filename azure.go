package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/RobustPerception/azure_metrics_exporter/config"
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
		Timeseries []struct {
			Data []struct {
				TimeStamp string  `json:"timeStamp"`
				Total     float64 `json:"total"`
				Average   float64 `json:"average"`
				Minimum   float64 `json:"minimum"`
				Maximum   float64 `json:"maximum"`
			} `json:"data"`
		} `json:"timeseries"`
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

type AzureResourceListResponse struct {
	Value []struct {
		Id        string `json:"id"`
		Name      string `json:"name"`
		Type      string `json:"type"`
		ManagedBy string `json:"managedBy"`
		Location  string `json:"location"`
	} `json:"value"`
}

// AzureClient represents our client to talk to the Azure api
type AzureClient struct {
	client               *http.Client
	accessToken          string
	accessTokenExpiresOn time.Time
}

// NewAzureClient returns an Azure client to talk the Azure API
func NewAzureClient() *AzureClient {
	return &AzureClient{
		client:               &http.Client{},
		accessToken:          "",
		accessTokenExpiresOn: time.Time{},
	}
}

func (ac *AzureClient) getAccessToken() error {
	target := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/token", sc.C.Credentials.TenantID)
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"resource":      {"https://management.azure.com/"},
		"client_id":     {sc.C.Credentials.ClientID},
		"client_secret": {sc.C.Credentials.ClientSecret},
	}
	resp, err := ac.client.PostForm(target, form)
	if err != nil {
		return fmt.Errorf("Error authenticating against Azure API: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("Did not get status code 200, got: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Error reading body of response: %v", err)
	}
	var data map[string]interface{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return fmt.Errorf("Error unmarshalling response body: %v", err)
	}
	ac.accessToken = data["access_token"].(string)
	expiresOn, err := strconv.ParseInt(data["expires_on"].(string), 10, 64)
	if err != nil {
		return fmt.Errorf("Error ParseInt of expires_on failed: %v", err)
	}
	ac.accessTokenExpiresOn = time.Unix(expiresOn, 0).UTC()

	return nil
}

// Returns metric definitions for all configured target and resource groups
func (ac *AzureClient) getMetricDefinitions() (map[string]AzureMetricDefinitionResponse, error) {

	definitions := make(map[string]AzureMetricDefinitionResponse)

	for _, target := range sc.C.Targets {
		def, err := ac.getAzureMetricDefinitionResponse(target.Resource)
		if err != nil {
			return nil, err
		}
		definitions[target.Resource] = *def
	}

	for _, resourceGroup := range sc.C.ResourceGroups {
		resources, err := ac.filteredListFromResourceGroup(resourceGroup)
		if err != nil {
			return nil, fmt.Errorf("Failed to get resources for resource group %s and resource types %s: %v",
				resourceGroup.ResourceGroup, resourceGroup.ResourceTypes, err)
		}
		for _, resource := range resources {
			def, err := ac.getAzureMetricDefinitionResponse(resource)
			if err != nil {
				return nil, err
			}
			definitions[resource] = *def
		}
	}

	return definitions, nil
}

// Returns AzureMetricDefinitionResponse for a given resource
func (ac *AzureClient) getAzureMetricDefinitionResponse(resource string) (*AzureMetricDefinitionResponse, error) {
	apiVersion := "2018-01-01"

	metricsResource := fmt.Sprintf("subscriptions/%s%s", sc.C.Credentials.SubscriptionID, resource)
	metricsTarget := fmt.Sprintf("https://management.azure.com/%s/providers/microsoft.insights/metricDefinitions?api-version=%s", metricsResource, apiVersion)
	req, err := http.NewRequest("GET", metricsTarget, nil)
	if err != nil {
		return nil, fmt.Errorf("Error creating HTTP request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+ac.accessToken)
	resp, err := ac.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Error: %v", err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Error reading body of response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Error: %v", string(body))
	}

	def := &AzureMetricDefinitionResponse{}
	err = json.Unmarshal(body, def)
	if err != nil {
		return nil, fmt.Errorf("Error unmarshalling response body: %v", err)
	}

	return def, nil
}

func (ac *AzureClient) getMetricValue(resource string, metricNames string, aggregations []string) (AzureMetricValueResponse, error) {
	apiVersion := "2018-01-01"

	metricsResource := fmt.Sprintf("subscriptions/%s%s", sc.C.Credentials.SubscriptionID, resource)
	endTime, startTime := GetTimes()

	metricValueEndpoint := fmt.Sprintf("https://management.azure.com/%s/providers/microsoft.insights/metrics", metricsResource)

	req, err := http.NewRequest("GET", metricValueEndpoint, nil)
	if err != nil {
		return AzureMetricValueResponse{}, fmt.Errorf("Error creating HTTP request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+ac.accessToken)

	values := url.Values{}
	if metricNames != "" {
		values.Add("metricnames", metricNames)
	}
	if len(aggregations) > 0 {
		values.Add("aggregation", strings.Join(aggregations, ","))
	} else {
		values.Add("aggregation", "Total,Average,Minimum,Maximum")
	}
	values.Add("timespan", fmt.Sprintf("%s/%s", startTime, endTime))
	values.Add("api-version", apiVersion)

	req.URL.RawQuery = values.Encode()

	resp, err := ac.client.Do(req)
	if err != nil {
		return AzureMetricValueResponse{}, fmt.Errorf("Error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return AzureMetricValueResponse{}, fmt.Errorf("Unable to query metrics API with status code: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return AzureMetricValueResponse{}, fmt.Errorf("Error reading body of response: %v", err)
	}

	var data AzureMetricValueResponse
	err = json.Unmarshal(body, &data)
	if err != nil {
		return AzureMetricValueResponse{}, fmt.Errorf("Error unmarshalling response body: %v", err)
	}

	if data.APIError.Code != "" {
		return AzureMetricValueResponse{}, fmt.Errorf("Metrics API returned error: %s - %v", data.APIError.Code, data.APIError.Message)
	}

	return data, nil
}

// Returns resource list resolved and filtered from resource_groups configuration
func (ac *AzureClient) filteredListFromResourceGroup(resourceGroup config.ResourceGroup) ([]string, error) {
	resources, err := ac.listFromResourceGroup(resourceGroup.ResourceGroup, resourceGroup.ResourceTypes)
	if err != nil {
		return nil, err
	}
	filteredResources := ac.filterResources(resources, resourceGroup)
	return filteredResources, nil
}

// Returns resource list filtered by tag name and tag value
func (ac *AzureClient) filteredListByTag(resourceTag config.ResourceTag) ([]string, error) {
	resources, err := ac.listByTag(resourceTag.ResourceTagName, resourceTag.ResourceTagValue)
	if err != nil {
		return nil, err
	}
	return resources, nil
}

// Returns all resources for given resource group and types
func (ac *AzureClient) listFromResourceGroup(resourceGroup string, resourceTypes []string) ([]string, error) {
	apiVersion := "2018-02-01"

	var filterTypesElements []string
	for _, filterType := range resourceTypes {
		filterTypesElements = append(filterTypesElements, fmt.Sprintf("resourcetype eq '%s'", filterType))
	}
	filterTypes := url.QueryEscape(strings.Join(filterTypesElements, " or "))

	subscription := fmt.Sprintf("subscriptions/%s", sc.C.Credentials.SubscriptionID)

	resourcesEndpoint := fmt.Sprintf("https://management.azure.com/%s/resourceGroups/%s/resources?api-version=%s&$filter=%s", subscription, resourceGroup, apiVersion, filterTypes)

	body, err := getAzureMonitorResponse(resourcesEndpoint)

	if err != nil {
		return nil, err
	}

	var data AzureResourceListResponse
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, fmt.Errorf("Error unmarshalling response body: %v", err)
	}

	resources := extractResourceNames(data, subscription)

	return resources, nil
}

// Returns all resource with the given couple tagname, tagvalue
func (ac *AzureClient) listByTag(tagName string, tagValue string) ([]string, error) {
	apiVersion := "2018-05-01"
	securedTagName := secureString(tagName)
	securedTagValue := secureString(tagValue)
	filterTypes := url.QueryEscape(fmt.Sprintf("tagName eq '%s' and tagValue eq '%s'", securedTagName, securedTagValue))

	subscription := fmt.Sprintf("subscriptions/%s", sc.C.Credentials.SubscriptionID)

	resourcesEndpoint := fmt.Sprintf("https://management.azure.com/%s/resources?api-version=%s&$filter=%s", subscription, apiVersion, filterTypes)

	body, err := getAzureMonitorResponse(resourcesEndpoint)

	if err != nil {
		return nil, err
	}

	var data AzureResourceListResponse
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, fmt.Errorf("Error unmarshalling response body: %v", err)
	}

	resources := extractResourceNames(data, subscription)

	return resources, nil
}

func secureString(value string) string {
	securedValue := strings.ReplaceAll(value, "'", "\\'")
	fmt.Printf("string returned{%s}", securedValue)
	return securedValue
}


func getAzureMonitorResponse(azureManagementEndpoint string) ([]byte, error) {
	req, err := http.NewRequest("GET", azureManagementEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("Error creating HTTP request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+ac.accessToken)

	resp, err := ac.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Error: %v", err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Unable to query API with status code: %d and with body: %s", resp.StatusCode, body)
	}

	if err != nil {
		return nil, fmt.Errorf("Error reading body of response: %v", err)
	}
	return body, err

}

// Extract resource names from the AzureResourceListResponse
func extractResourceNames(data AzureResourceListResponse, subscription string) []string {
	var resources []string
	for _, result := range data.Value {
		// subscription + leading '/'
		subscriptionPrefixLen := len(subscription) + 1

		// remove subscription from path to match manually specified ones
		resources = append(resources, result.Id[subscriptionPrefixLen:])
	}
	return resources
}

// Returns a filtered resource list based on a given resource list and regular expressions from the configuration
func (ac *AzureClient) filterResources(resources []string, resourceGroup config.ResourceGroup) []string {
	filteredResources := []string{}

	for _, resource := range resources {
		resourceParts := strings.Split(resource, "/")
		resourceName := resourceParts[len(resourceParts)-1]

		if len(resourceGroup.ResourceNameIncludeRe) != 0 {
			include := false
			for _, rx := range resourceGroup.ResourceNameIncludeRe {
				if rx.MatchString(resourceName) {
					include = true
					break
				}
			}

			if !include {
				continue
			}
		}

		exclude := false
		for _, rx := range resourceGroup.ResourceNameExcludeRe {
			if rx.MatchString(resourceName) {
				exclude = true
				break
			}
		}

		if exclude {
			continue
		}

		filteredResources = append(filteredResources, resource)
	}

	return filteredResources
}

func (ac *AzureClient) refreshAccessToken() error {
	now := time.Now().UTC()
	refreshAt := ac.accessTokenExpiresOn.Add(-10 * time.Minute)
	if now.After(refreshAt) {
		err := ac.getAccessToken()
		if err != nil {
			return fmt.Errorf("Error refreshing access token: %v", err)
		}
	}

	return nil
}
