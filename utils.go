package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// PrintPrettyJSON - Prints structs nicely for debugging.
func PrintPrettyJSON(input map[string]interface{}) {
	out, err := json.MarshalIndent(input, "", "\t")
	if err != nil {
		log.Fatalf("Error indenting JSON: %v", err)
	}
	fmt.Println(string(out))
}

// GetTimes - Returns the current time and the time 1 minute ago.
func GetTimes() (string, string) {
	now := time.Now().Format(time.RFC3339)
	oneMinute := time.Minute * time.Duration(-1)
	then := time.Now().Add(oneMinute).Format(time.RFC3339)
	return now, then
}

// CreateResourceLabels - Returns resource labels for a give resource ID.
func CreateResourceLabels(resourceID string) map[string]string {
	labels := make(map[string]string)
	labels["resource_group"] = strings.Split(resourceID, "/")[4]
	labels["resource_name"] = strings.Split(resourceID, "/")[8]
	return labels
}
