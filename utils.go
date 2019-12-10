package main

import (
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"regexp"
	"strings"
	"time"
)

var (
	// resource component positions in a ResourceURL
	resourceGroupPosition      = 4
	resourceNamePosition       = 8
	subResourceNamePosition    = 10
	resourceTypeSuffixPosition = 9
	resourceTypePosition       = 7
	resourceTypePrefixPosition = 6
	invalidLabelChars          = regexp.MustCompile(`[^a-zA-Z0-9_]+`)
)

// PrintPrettyJSON - Prints structs nicely for debugging.
func PrintPrettyJSON(input map[string]interface{}) {
	out, err := json.MarshalIndent(input, "", "\t")
	if err != nil {
		log.Fatalf("Error indenting JSON: %v", err)
	}
	fmt.Println(string(out))
}

// GetTimes - Returns the endTime and startTime used for querying Azure Metrics API
func GetTimes() (string, string) {
	// Make sure we are using UTC
	now := time.Now().UTC()

	// Use query delay of 3 minutes when querying for latest metric data
	endTime := now.Add(time.Minute * time.Duration(-3)).Format(time.RFC3339)
	startTime := now.Add(time.Minute * time.Duration(-4)).Format(time.RFC3339)
	return endTime, startTime
}

// CreateResourceLabels - Returns resource labels for a given resource URL.
func CreateResourceLabels(resourceURL string) map[string]string {
	labels := make(map[string]string)
	resource := strings.Split(resourceURL, "/")

	labels["resource_group"] = resource[resourceGroupPosition]
	labels["resource_name"] = resource[resourceNamePosition]
	if len(resource) > 13 {
		labels["sub_resource_name"] = resource[subResourceNamePosition]
	}
	return labels
}

// GetResourceType returns the resource type with the namespace
func GetResourceType(resourceURL string) string {
	resource := strings.Split(resourceURL, "/")
	var str strings.Builder
	str.WriteString(resource[resourceTypePrefixPosition])
	str.WriteString("/")
	str.WriteString(resource[resourceTypePosition])
	if len(resource) > 13 {
		str.WriteString("/")
		str.WriteString(resource[resourceTypeSuffixPosition])
	}
	return str.String()
}

func CreateAllResourceLabelsFrom(rm resourceMeta) map[string]string {
	formatTag := "pretty"
	labels := make(map[string]string)

	for k, v := range rm.resource.Tags {
		k = strings.ToLower(k)
		k = "tag_" + k
		k = invalidLabelChars.ReplaceAllString(k, "_")
		labels[k] = v
	}

	// create a label for each field of the resource
	val := reflect.ValueOf(rm.resource)
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		tag := reflect.TypeOf(rm.resource).Field(i).Tag.Get(formatTag)
		if field.Kind() == reflect.String {
			labels[tag] = field.String()
		}
	}

	// Most labels are handled by iterating over the fields of resourceMeta.AzureResource.
	// Their tag values are used as label keys.
	// To keep coherence with the metric labels, we create "resource_group",  "resource_name"
	// and "sub_resource_name" by invoking CreateResourceLabels.
	resourceLabels := CreateResourceLabels(rm.resourceURL)
	for k, v := range resourceLabels {
		labels[k] = v
	}
	return labels
}

func hasAggregation(aggregations []string, aggregation string) bool {
	if len(aggregations) == 0 {
		return true
	}

	for _, aggr := range aggregations {
		if aggr == aggregation {
			return true
		}
	}
	return false
}

func filterAggregations(aggregations []string) []string {
	base := []string{"Total", "Average", "Minimum", "Maximum"}
	if len(aggregations) > 0 {
		return aggregations
	}
	return base
}
