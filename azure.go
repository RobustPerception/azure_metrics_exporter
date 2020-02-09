package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/RobustPerception/azure_metrics_exporter/config"
)

var (
	apiVersionDate = regexp.MustCompile("^\\d{4}-\\d{2}-\\d{2}")
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

// MetricNamespaceCollectionResponse represents metric namespace response for a given resource from Azure.
type MetricNamespaceCollectionResponse struct {
	MetricNamespaceCollection []metricNamespaceResponse `json:"value"`
}

type metricNamespaceResponse struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Type           string `json:"type"`
	Classification string `json:"classification"`
	Properties     struct {
		MetricNamespaceName string `json:"metricNamespaceName"`
	} `json:"properties"`
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

type AzureBatchMetricResponse struct {
	Responses []struct {
		HttpStatusCode int                      `json:"httpStatusCode"`
		Content        AzureMetricValueResponse `json:"content"`
	} `json:"responses"`
}

type AzureBatchLookupResponse struct {
	Responses []struct {
		HttpStatusCode int           `json:"httpStatusCode"`
		Content        AzureResource `json:"content"`
	} `json:"responses"`
}

type AzureResourceListResponse struct {
	Value []AzureResource `json:"value"`
}

type AzureResource struct {
	ID           string            `json:"id" pretty:"id"`
	Name         string            `json:"name" pretty:"resource_name"`
	Location     string            `json:"location" pretty:"azure_location"`
	Type         string            `json:"type" pretty:"resource_type"`
	Tags         map[string]string `json:"tags" pretty:"tags"`
	ManagedBy    string            `json:"managedBy" pretty:"managed_by"`
	Subscription string            `pretty:"azure_subscription"`
}

type APIVersionResponse struct {
	Value []struct {
		ID            string `json:"id"`
		Namespace     string `json:"namespace"`
		ResourceTypes []struct {
			ResourceType string   `json:"resourceType"`
			Locations    []string `json:"locations"`
			APIVersions  []string `json:"apiVersions"`
		} `json:"resourceTypes"`
		RegistrationState string `json:"registrationState"`
	} `json:"value"`
}

type APIVersionData struct {
	Endpoint string
	Date     time.Time
}

type APIVersionMap map[string]string

func latestVersionFrom(apiList []string) string {
	var latest = &APIVersionData{}
	format := "2006-01-02"

	for _, api := range apiList {
		dateStr := apiVersionDate.FindString(api)
		date, err := time.Parse(format, dateStr)
		if err != nil {
			log.Println(err)
			continue
		}

		if latest == nil || latest.Date.Before(date) {
			latest = &APIVersionData{Endpoint: api, Date: date}
		}

	}
	return latest.Endpoint
}

func (r *APIVersionResponse) extractAPIVersions() APIVersionMap {
	var apiVersions = APIVersionMap{}
	for _, val := range r.Value {
		for _, t := range val.ResourceTypes {
			if len(t.APIVersions) == 0 {
				continue
			}
			resourceType := strings.Join([]string{val.Namespace, t.ResourceType}, "/")
			apiVersions[resourceType] = latestVersionFrom(t.APIVersions)
		}
	}
	return apiVersions
}

func (m *APIVersionMap) findBy(resourceType string) string {
	var apiVersion string
	for mType, mVersion := range *m {
		if mType == resourceType {
			apiVersion = mVersion
			break
		}
	}
	return apiVersion
}

// AzureClient represents our client to talk to the Azure api
type AzureClient struct {
	client               *http.Client
	accessToken          string
	accessTokenExpiresOn time.Time
	APIVersions          APIVersionMap
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
	var resp *http.Response
	var err error
	if len(sc.C.Credentials.ClientID) == 0 {
		log.Printf("Using managed identity")
		target := fmt.Sprintf("http://169.254.169.254/metadata/identity/oauth2/token?resource=%s&api-version=2018-02-01", sc.C.ResourceManagerURL)
		req, err := http.NewRequest("GET", target, nil)
		if err != nil {
			return fmt.Errorf("Error getting token against Azure MSI endpoint: %v", err)
		}
		req.Header.Add("Metadata", "true")
		resp, err = ac.client.Do(req)
	} else {
		target := fmt.Sprintf("%s/%s/oauth2/token", sc.C.ActiveDirectoryAuthorityURL, sc.C.Credentials.TenantID)
		form := url.Values{
			"grant_type":    {"client_credentials"},
			"resource":      {sc.C.ResourceManagerURL},
			"client_id":     {sc.C.Credentials.ClientID},
			"client_secret": {sc.C.Credentials.ClientSecret},
		}
		resp, err = ac.client.PostForm(target, form)
	}
	if err != nil {
		return fmt.Errorf("Error authenticating against Azure API: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		respBytest, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("Did not get status code 200, got: %d with body: %s", resp.StatusCode, string(respBytest))
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
		def, err := ac.getAzureMetricDefinitionResponse(target.Resource, target.MetricNamespace)
		if err != nil {
			return nil, err
		}
		defKey := target.Resource
		if len(target.MetricNamespace) > 0 {
			defKey = fmt.Sprintf("%s (Metric namespace: %s)", defKey, target.MetricNamespace)
		}
		definitions[defKey] = *def
	}

	for _, resourceGroup := range sc.C.ResourceGroups {
		resources, err := ac.filteredListFromResourceGroup(resourceGroup)
		if err != nil {
			return nil, fmt.Errorf("Failed to get resources for resource group %s and resource types %s: %v",
				resourceGroup.ResourceGroup, resourceGroup.ResourceTypes, err)
		}
		for _, resource := range resources {
			def, err := ac.getAzureMetricDefinitionResponse(resource.ID, resourceGroup.MetricNamespace)
			if err != nil {
				return nil, err
			}
			defKey := resource.ID
			if len(resourceGroup.MetricNamespace) > 0 {
				defKey = fmt.Sprintf("%s (Metric namespace: %s)", defKey, resourceGroup.MetricNamespace)
			}
			definitions[defKey] = *def
		}
	}
	return definitions, nil
}

// Returns metric namespaces for all configured target and resource groups.
func (ac *AzureClient) getMetricNamespaces() (map[string]MetricNamespaceCollectionResponse, error) {
	namespaces := make(map[string]MetricNamespaceCollectionResponse)
	for _, target := range sc.C.Targets {
		namespaceCollection, err := ac.getMetricNamespaceCollectionResponse(target.Resource)
		if err != nil {
			return nil, err
		}
		namespaces[target.Resource] = *namespaceCollection
	}

	for _, resourceGroup := range sc.C.ResourceGroups {
		resources, err := ac.filteredListFromResourceGroup(resourceGroup)
		if err != nil {
			return nil, fmt.Errorf("Failed to get resources for resource group %s and resource types %s: %v",
				resourceGroup.ResourceGroup, resourceGroup.ResourceTypes, err)
		}
		for _, resource := range resources {
			namespaceCollection, err := ac.getMetricNamespaceCollectionResponse(resource.ID)
			if err != nil {
				return nil, err
			}
			namespaces[resource.ID] = *namespaceCollection
		}
	}
	return namespaces, nil
}

// Returns AzureMetricDefinitionResponse for a given resource
func (ac *AzureClient) getAzureMetricDefinitionResponse(resource string, metricNamespace string) (*AzureMetricDefinitionResponse, error) {
	apiVersion := "2018-01-01"

	metricsResource := fmt.Sprintf("subscriptions/%s%s", sc.C.Credentials.SubscriptionID, resource)
	metricsTarget := fmt.Sprintf("%s/%s/providers/microsoft.insights/metricDefinitions?api-version=%s", sc.C.ResourceManagerURL, metricsResource, apiVersion)
	if metricNamespace != "" {
		metricsTarget = fmt.Sprintf("%s&metricnamespace=%s", metricsTarget, url.QueryEscape(metricNamespace))
	}

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

// Returns MetricNamespaceCollectionResponse for a given resource
func (ac *AzureClient) getMetricNamespaceCollectionResponse(resource string) (*MetricNamespaceCollectionResponse, error) {
	apiVersion := "2017-12-01-preview"

	nsResource := fmt.Sprintf("subscriptions/%s%s", sc.C.Credentials.SubscriptionID, resource)
	nsTarget := fmt.Sprintf("%s/%s/providers/microsoft.insights/metricNamespaces?api-version=%s", sc.C.ResourceManagerURL, nsResource, apiVersion)
	req, err := http.NewRequest("GET", nsTarget, nil)
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

	namespaceCollection := &MetricNamespaceCollectionResponse{}
	err = json.Unmarshal(body, namespaceCollection)
	if err != nil {
		return nil, fmt.Errorf("Error unmarshalling response body: %v", err)
	}
	return namespaceCollection, nil
}

// Returns resource list resolved and filtered from resource_groups configuration
func (ac *AzureClient) filteredListFromResourceGroup(resourceGroup config.ResourceGroup) ([]AzureResource, error) {
	resources, err := ac.listFromResourceGroup(resourceGroup.ResourceGroup, resourceGroup.ResourceTypes)
	if err != nil {
		return nil, err
	}
	filteredResources := ac.filterResources(resources, resourceGroup)

	return filteredResources, nil
}

// Returns resource list filtered by tag name and tag value
func (ac *AzureClient) filteredListByTag(resourceTag config.ResourceTag, resourcesMap map[string][]byte) ([]AzureResource, error) {
	resources, err := ac.listByTag(resourceTag.ResourceTagName, resourceTag.ResourceTagValue, resourceTag.ResourceTypes, resourcesMap)
	if err != nil {
		return nil, err
	}
	return resources, nil
}

// Returns all resources for given resource group and types
func (ac *AzureClient) listFromResourceGroup(resourceGroup string, resourceTypes []string) ([]AzureResource, error) {
	apiVersion := "2018-02-01"

	var filterTypesElements []string
	for _, filterType := range resourceTypes {
		filterTypesElements = append(filterTypesElements, fmt.Sprintf("resourcetype eq '%s'", filterType))
	}
	filterTypes := url.QueryEscape(strings.Join(filterTypesElements, " or "))
	subscription := fmt.Sprintf("subscriptions/%s", sc.C.Credentials.SubscriptionID)
	resourcesEndpoint := fmt.Sprintf("%s/%s/resourceGroups/%s/resources?api-version=%s&$filter=%s", sc.C.ResourceManagerURL, subscription, resourceGroup, apiVersion, filterTypes)

	body, err := getAzureMonitorResponse(resourcesEndpoint)
	if err != nil {
		return nil, err
	}

	var data AzureResourceListResponse
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, fmt.Errorf("Error unmarshalling response body: %v", err)
	}
	return data.extendResources(), nil
}

// Returns all resource with the given couple tagname, tagvalue
func (ac *AzureClient) listByTag(tagName string, tagValue string, types []string, resourcesMap map[string][]byte) ([]AzureResource, error) {
	apiVersion := "2018-05-01"
	securedTagName := secureString(tagName)
	securedTagValue := secureString(tagValue)
	filterTypes := url.QueryEscape(fmt.Sprintf("tagName eq '%s' and tagValue eq '%s'", securedTagName, securedTagValue))
	subscription := fmt.Sprintf("subscriptions/%s", sc.C.Credentials.SubscriptionID)
	resourcesEndpoint := fmt.Sprintf("%s/%s/resources?api-version=%s&$filter=%s", sc.C.ResourceManagerURL, subscription, apiVersion, filterTypes)

	body, ok := resourcesMap[resourcesEndpoint]
	if !ok {
		var err error
		body, err = getAzureMonitorResponse(resourcesEndpoint)
		if err != nil {
			return nil, err
		}
		resourcesMap[resourcesEndpoint] = body
	}

	var data AzureResourceListResponse
	err := json.Unmarshal(body, &data)
	if err != nil {
		return nil, fmt.Errorf("Error unmarshalling response body: %v", err)
	}

	if len(types) > 0 {
		data.Value = data.filterTypesInResourceList(types)
	}
	return data.extendResources(), nil
}

func (ac *AzureClient) listAPIVersions() error {
	apiVersion := "2019-05-10"
	var versionResponse APIVersionResponse

	subscription := fmt.Sprintf("subscriptions/%s", sc.C.Credentials.SubscriptionID)
	resourcesEndpoint := fmt.Sprintf("%s/%s/providers?api-version=%s", sc.C.ResourceManagerURL, subscription, apiVersion)

	body, err := getAzureMonitorResponse(resourcesEndpoint)
	if err != nil {
		return err
	}

	err = json.Unmarshal(body, &versionResponse)
	if err != nil {
		return fmt.Errorf("Error unmarshalling response body: %v", err)
	}

	ac.APIVersions = versionResponse.extractAPIVersions()
	return nil
}

func (response *AzureResourceListResponse) filterTypesInResourceList(types []string) []AzureResource {
	typesMap := make(map[string]struct{})
	for _, resourceType := range types {
		typesMap[resourceType] = struct{}{}
	}
	var filteredResources []AzureResource
	for _, resource := range response.Value {
		if _, typeExist := typesMap[resource.Type]; typeExist {
			filteredResources = append(filteredResources, resource)
		}
	}
	return filteredResources
}

func secureString(value string) string {
	securedValue := strings.Replace(value, "'", "\\'", -1)
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

func (ar *AzureResourceListResponse) extendResources() []AzureResource {
	subscription := fmt.Sprintf("subscriptions/%s", sc.C.Credentials.SubscriptionID)
	var subscriptionPrefixLen = len(subscription) + 1

	for i, val := range ar.Value {
		ar.Value[i].ID = val.ID[subscriptionPrefixLen:]
		ar.Value[i].Subscription = sc.C.Credentials.SubscriptionID
	}
	return ar.Value
}

// Returns a filtered resource list based on a given resource list and regular expressions from the configuration
func (ac *AzureClient) filterResources(resources []AzureResource, resourceGroup config.ResourceGroup) []AzureResource {
	filteredResources := []AzureResource{}

	for _, resource := range resources {
		if len(resourceGroup.ResourceNameIncludeRe) != 0 {
			include := false
			for _, rx := range resourceGroup.ResourceNameIncludeRe {
				if rx.MatchString(resource.Name) {
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
			if rx.MatchString(resource.Name) {
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

type batchBody struct {
	Requests []batchRequest `json:"requests"`
}

type batchRequest struct {
	RelativeURL string `json:"relativeUrl"`
	Method      string `json:"httpMethod"`
}

func resourceURLFrom(resource string, metricNamespace string, metricNames string, aggregations []string) string {
	apiVersion := "2018-01-01"

	path := fmt.Sprintf(
		"/subscriptions/%s%s/providers/microsoft.insights/metrics",
		sc.C.Credentials.SubscriptionID,
		resource,
	)

	endTime, startTime := GetTimes()

	values := url.Values{}
	if metricNames != "" {
		values.Add("metricnames", metricNames)
	}
	if metricNamespace != "" {
		values.Add("metricnamespace", metricNamespace)
	}
	filtered := filterAggregations(aggregations)
	values.Add("aggregation", strings.Join(filtered, ","))
	values.Add("timespan", fmt.Sprintf("%s/%s", startTime, endTime))
	values.Add("api-version", apiVersion)

	url := url.URL{
		Path:     path,
		RawQuery: values.Encode(),
	}
	return url.String()
}

func (ac *AzureClient) getBatchResponseBody(urls []string) ([]byte, error) {

	rmBaseURL := sc.C.ResourceManagerURL
	if !strings.HasSuffix(sc.C.ResourceManagerURL, "/") {
		rmBaseURL += "/"
	}

	apiURL := fmt.Sprintf("%sbatch?api-version=2017-03-01", rmBaseURL)

	batch := batchBody{}
	for _, u := range urls {
		batch.Requests = append(batch.Requests, batchRequest{
			RelativeURL: u,
			Method:      "GET",
		})
	}

	batchJSON, err := json.Marshal(batch)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(batchJSON))
	if err != nil {
		return nil, fmt.Errorf("Error creating HTTP request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ac.accessToken)

	resp, err := ac.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Error: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}
