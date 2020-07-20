package config

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"
	"sync"

	yaml "gopkg.in/yaml.v2"
)

// Config - Azure exporter configuration
type Config struct {
	ActiveDirectoryAuthorityURL string          `yaml:"active_directory_authority_url"`
	ResourceManagerURL          string          `yaml:"resource_manager_url"`
	Credentials                 Credentials     `yaml:"credentials"`
	Targets                     []Target        `yaml:"targets"`
	ResourceGroups              []ResourceGroup `yaml:"resource_groups"`
	ResourceTags                []ResourceTag   `yaml:"resource_tags"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// SafeConfig - mutex protected config for live reloads.
type SafeConfig struct {
	sync.RWMutex
	C *Config
}

// ReloadConfig - allows for live reloads of the configuration file.
func (sc *SafeConfig) ReloadConfig(confFile string) (err error) {
	var c = &Config{
		ActiveDirectoryAuthorityURL: "https://login.microsoftonline.com/",
		ResourceManagerURL:          "https://management.azure.com/",
	}

	yamlFile, err := ioutil.ReadFile(confFile)
	if err != nil {
		return fmt.Errorf("Error reading config file: %s", err)
	}

	if err := yaml.Unmarshal(yamlFile, c); err != nil {
		return fmt.Errorf("Error parsing config file: %s", err)
	}

	if err := c.Validate(); err != nil {
		return fmt.Errorf("Error validating config file: %s", err)
	}

	sc.Lock()
	sc.C = c
	sc.Unlock()

	return nil
}

var validAggregations = []string{"Total", "Average", "Minimum", "Maximum"}

func (c *Config) Validate() (err error) {
	for _, t := range c.Targets {
		if err := c.validateAggregations(t.Aggregations); err != nil {
			return err
		}

		if len(t.Resource) == 0 {
			return fmt.Errorf("name needs to be specified in each resource")
		}

		if !strings.HasPrefix(t.Resource, "/") {
			return fmt.Errorf("Resource path %q must start with a /", t.Resource)
		}

		if len(t.Metrics) == 0 {
			return fmt.Errorf("At least one metric needs to be specified in each resource")
		}
	}

	for _, t := range c.ResourceGroups {
		if err := c.validateAggregations(t.Aggregations); err != nil {
			return err
		}

		if len(t.ResourceGroup) == 0 {
			return fmt.Errorf("resource_group needs to be specified in each resource group")
		}

		if len(t.ResourceTypes) == 0 {
			return fmt.Errorf("At lease one resource type needs to be specified in each resource group")
		}

		if len(t.Metrics) == 0 {
			return fmt.Errorf("At least one metric needs to be specified in each resource group")
		}
	}

	for _, t := range c.ResourceTags {
		if err := c.validateAggregations(t.Aggregations); err != nil {
			return err
		}

		if len(t.ResourceTagName) == 0 {
			return fmt.Errorf("resource_tag_name needs to be specified in each resource tag")
		}

		if len(t.ResourceTagValue) == 0 {
			return fmt.Errorf("resource_tag_value needs to be specified in each resource tag")
		}

		if len(t.Metrics) == 0 {
			return fmt.Errorf("At least one metric needs to be specified in each resource tag")
		}
	}

	return nil
}

func (c *Config) validateAggregations(aggregations []string) error {
	for _, a := range aggregations {
		ok := false
		for _, valid := range validAggregations {
			if a == valid {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("%s is not one of the valid aggregations (%v)", a, validAggregations)
		}
	}

	return nil
}

// Credentials - Azure credentials
type Credentials struct {
	SubscriptionID string `yaml:"subscription_id"`
	ClientID       string `yaml:"client_id"`
	ClientSecret   string `yaml:"client_secret"`
	TenantID       string `yaml:"tenant_id"`

	XXX map[string]interface{} `yaml:",inline"`
}

// Target represents Azure target resource and its associated metric definitions
type Target struct {
	Resource        string   `yaml:"resource"`
	MetricNamespace string   `yaml:"metric_namespace"`
	Metrics         []Metric `yaml:"metrics"`
	Aggregations    []string `yaml:"aggregations"`

	XXX map[string]interface{} `yaml:",inline"`
}

// ResourceGroup represents Azure target resource group and its associated metric definitions
type ResourceGroup struct {
	ResourceGroup         string   `yaml:"resource_group"`
	MetricNamespace       string   `yaml:"metric_namespace"`
	ResourceTypes         []string `yaml:"resource_types"`
	ResourceNameIncludeRe []Regexp `yaml:"resource_name_include_re"`
	ResourceNameExcludeRe []Regexp `yaml:"resource_name_exclude_re"`
	Metrics               []Metric `yaml:"metrics"`
	Aggregations          []string `yaml:"aggregations"`

	XXX map[string]interface{} `yaml:",inline"`
}

// ResourceTag selects resources with tag name and tag value
type ResourceTag struct {
	ResourceTagName  string   `yaml:"resource_tag_name"`
	ResourceTagValue string   `yaml:"resource_tag_value"`
	MetricNamespace  string   `yaml:"metric_namespace"`
	ResourceTypes    []string `yaml:"resource_types"`
	Metrics          []Metric `yaml:"metrics"`
	Aggregations     []string `yaml:"aggregations"`

	XXX map[string]interface{} `yaml:",inline"`
}

// Metric defines metric name
type Metric struct {
	Name string `yaml:"name"`

	XXX map[string]interface{} `yaml:",inline"`
}

// Regexp encapsulates a regexp.Regexp and makes it YAML marshalable.
type Regexp struct {
	*regexp.Regexp
}

func checkOverflow(m map[string]interface{}, ctx string) error {
	if len(m) > 0 {
		var keys []string
		for k := range m {
			keys = append(keys, k)
		}
		return fmt.Errorf("unknown fields in %s: %s", ctx, strings.Join(keys, ", "))
	}
	return nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (s *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain Config
	if err := unmarshal((*plain)(s)); err != nil {
		return err
	}
	if err := checkOverflow(s.XXX, "config"); err != nil {
		return err
	}
	return nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (s *Credentials) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain Credentials
	if err := unmarshal((*plain)(s)); err != nil {
		return err
	}
	if err := checkOverflow(s.XXX, "config"); err != nil {
		return err
	}
	return nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (s *Metric) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain Metric
	if err := unmarshal((*plain)(s)); err != nil {
		return err
	}
	if err := checkOverflow(s.XXX, "config"); err != nil {
		return err
	}
	return nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (s *Target) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain Target
	if err := unmarshal((*plain)(s)); err != nil {
		return err
	}
	if err := checkOverflow(s.XXX, "config"); err != nil {
		return err
	}
	return nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (s *ResourceGroup) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain ResourceGroup
	if err := unmarshal((*plain)(s)); err != nil {
		return err
	}
	if err := checkOverflow(s.XXX, "config"); err != nil {
		return err
	}
	return nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (re *Regexp) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	regex, err := regexp.Compile("^(?:" + s + ")$")
	if err != nil {
		return err
	}
	re.Regexp = regex
	return nil
}
