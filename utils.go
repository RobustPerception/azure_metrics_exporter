package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode"
)

// PrintPrettyJSON - Prints structs nicely for debugging.
func PrintPrettyJSON(input map[string]interface{}) {
	out, err := json.MarshalIndent(input, "", "\t")
	if err != nil {
		log.Fatalf("Error indenting JSON: %v", err)
	}
	fmt.Println(string(out))
}

// GetTimes - Returns the current time and the time 10 minutes ago.
func GetTimes() (string, string) {
	now := time.Now().Format(time.RFC3339)
	tenMinutes := time.Minute * time.Duration(-10)
	then := time.Now().Add(tenMinutes).Format(time.RFC3339)
	return now, then
}

// CreateResourceLabels - Returns resource labels for a give resource ID.
func CreateResourceLabels(resourceID string) map[string]string {
	labels := make(map[string]string)
	labels["resource_type"] = strings.Split(resourceID, "/")[6]
	labels["resource_group"] = strings.Split(resourceID, "/")[4]
	labels["resource_name"] = strings.Split(resourceID, "/")[8]
	return labels
}

// ToSnakeCase convert the given string to snake case following the Golang format:
// acronyms are converted to lower-case and preceded by an underscore.
func ToSnakeCase(in string) string {
	runes := []rune(in)
	var out []rune
	for i := 0; i < len(runes); i++ {
		if i > 0 && (unicode.IsUpper(runes[i]) || unicode.IsNumber(runes[i])) && ((i+1 < len(runes) && unicode.IsLower(runes[i+1])) || unicode.IsLower(runes[i-1])) {
			out = append(out, '_')
		}
		out = append(out, unicode.ToLower(runes[i]))
	}
	return string(out)
}
