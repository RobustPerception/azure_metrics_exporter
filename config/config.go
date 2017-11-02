package config

import (
	"fmt"
	"io/ioutil"
	"strings"
	"sync"

	yaml "gopkg.in/yaml.v2"
)

// Config - Azure exporter configuration
type Config struct {
	Credentials Credentials `yaml:"credentials"`
	Targets     []Target    `yaml:"targets"`

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
	var c = &Config{}

	yamlFile, err := ioutil.ReadFile(confFile)
	if err != nil {
		return fmt.Errorf("Error reading config file: %s", err)
	}

	if err := yaml.Unmarshal(yamlFile, c); err != nil {
		return fmt.Errorf("Error parsing config file: %s", err)
	}

	sc.Lock()
	sc.C = c
	sc.Unlock()

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
	Resource string   `yaml:"resource"`
	Metrics  []Metric `yaml:"metrics"`

	XXX map[string]interface{} `yaml:",inline"`
}

// Metric defines metric name
type Metric struct {
	Name string `yaml:"name"`

	XXX map[string]interface{} `yaml:",inline"`
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
