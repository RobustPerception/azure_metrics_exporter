package main

import (
	"reflect"
	"testing"
)

func TestCreateResourceLabels(t *testing.T) {
	var cases = []struct {
		url  string
		want map[string]string
	}{
		{
			"/subscriptions/abc123d4-e5f6-g7h8-i9j10-a1b2c3d4e5f6/resourceGroups/prod-rg-001/providers/Microsoft.Compute/virtualMachines/prod-vm-01/providers/microsoft.insights/metrics",
			map[string]string{"resource_group": "prod-rg-001", "resource_name": "prod-vm-01"},
		},
		{
			"/subscriptions/abc123d4-e5f6-g7h8-i9j10-a1b2c3d4e5f6/resourceGroups/prod-rg-002/providers/Microsoft.Sql/servers/sqlprod/databases/prod-db-01/providers/microsoft.insights/metrics",
			map[string]string{"resource_group": "prod-rg-002", "resource_name": "sqlprod", "sub_resource_name": "prod-db-01"},
		},
	}

	for _, c := range cases {
		got := CreateResourceLabels(c.url)

		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("doesn't create expected resource labels\ngot: %v\nwant: %v", got, c.want)
		}
	}
}

func TestCreateAllResourceLabelsFrom(t *testing.T) {
	var cases = []struct {
		rm   resourceMeta
		want map[string]string
	}{
		{
			resourceMeta{
				resourceURL: "/subscriptions/abc123d4-e5f6-g7h8-i9j10-a1b2c3d4e5f6/resourceGroups/prod-rg-001/providers/Microsoft.Compute/virtualMachines/prod-vm-01/providers/microsoft.insights/metrics",
				resource: AzureResource{
					ID:           "/resourceGroups/prod-rg-001/providers/Microsoft.Compute/virtualMachines/prod-vm-01",
					Name:         "fxpromdev01",
					Location:     "canadaeast",
					Type:         "Microsoft.Compute/virtualMachines",
					Tags:         map[string]string{"monitoring": "enabled", "department": "secret"},
					Subscription: "abc123d4-e5f6-g7h8-i9j10-a1b2c3d4e5f6",
				},
			},
			map[string]string{
				"azure_location":     "canadaeast",
				"azure_subscription": "abc123d4-e5f6-g7h8-i9j10-a1b2c3d4e5f6",
				"id":                 "/resourceGroups/prod-rg-001/providers/Microsoft.Compute/virtualMachines/prod-vm-01",
				"managed_by":         "",
				"resource_group":     "prod-rg-001",
				"resource_name":      "prod-vm-01",
				"resource_type":      "Microsoft.Compute/virtualMachines",
				"tag_department":     "secret",
				"tag_monitoring":     "enabled",
			},
		},
	}

	for _, c := range cases {
		got := CreateAllResourceLabelsFrom(c.rm)

		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("doesn't create expected resource labels\ngot: %v\nwant: %v", got, c.want)
		}
	}
}

func TestCetResourceType(t *testing.T) {
	var cases = []struct {
		url  string
		want string
	}{
		{
			"/subscriptions/abc123d4-e5f6-g7h8-i9j10-a1b2c3d4e5f6/resourceGroups/prod-rg-001/providers/Microsoft.Compute/virtualMachines/prod-vm-01/providers/microsoft.insights/metrics",
			"Microsoft.Compute/virtualMachines",
		},
		{
			"/subscriptions/abc123d4-e5f6-g7h8-i9j10-a1b2c3d4e5f6/resourceGroups/prod-rg-002/providers/Microsoft.Sql/servers/sqlprod/databases/prod-db-01/providers/microsoft.insights/metrics",
			"Microsoft.Sql/servers/databases",
		},
	}

	for _, c := range cases {
		got := GetResourceType(c.url)

		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("doesn't create expected resource type\ngot: %v\nwant: %v", got, c.want)
		}
	}
}
