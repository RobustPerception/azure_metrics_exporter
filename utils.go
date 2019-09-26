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
	resourceGroupPosition   = 4
	resourceNamePosition    = 8
	subResourceNamePosition = 10

	invalidLabelPrefix = regexp.MustCompile(`^[^a-zA-Z_]*`)
	invalidLabelChars  = regexp.MustCompile(`[^a-zA-Z0-9_]+`)
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

// CreateResourceLabels - Returns resource labels for a give resource ID.
func CreateResourceLabels(resourceID string) map[string]string {
	labels := make(map[string]string)
	resource := strings.Split(resourceID, "/")
	labels["resource_group"] = resource[resourceGroupPosition]
	labels["resource_name"] = resource[resourceNamePosition]
	if len(resource) > 13 {
		labels["sub_resource_name"] = resource[subResourceNamePosition]
	}

	return labels
}

func CreateAllResourceLabelsFrom(rm resourceMeta) map[string]string {
	formatTag := "pretty"
	labels := make(map[string]string)
	split := strings.Split(rm.resourceURL, "/")
	labels["resource_group"] = split[resourceGroupPosition]

	for k, v := range rm.resource.Tags {
		k = strings.ToLower(k)

		if !invalidLabelPrefix.MatchString(k) {
			k = "_" + k
		}

		k = invalidLabelChars.ReplaceAllString(k, "_")
		labels[k] = v
	}

	if len(split) > 13 {
		labels["sub_resource_name"] = split[subResourceNamePosition]
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
