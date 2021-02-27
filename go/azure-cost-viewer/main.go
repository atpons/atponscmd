package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/costmanagement/mgmt/costmanagement"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/Azure/go-autorest/autorest/date"
	"github.com/jinzhu/now"
	"github.com/olekukonko/tablewriter"
	"go.uber.org/multierr"
)

const (
	AzureSubscriptionIdEnvKey = "AZURE_SUBSCRIPTION_ID"
	AzureTenantIdEnvKey       = "AZURE_TENANT_ID"
	AzureClientIdEnvKey       = "AZURE_CLIENT_ID"
	AzureClientSecretEnvKey   = "AZURE_CLIENT_SECRET"
)

var Global *Config

type Config struct {
	AzureSubscriptionId string
	AzureTenantId       string
	AzureClientId       string
	AzureClientSecret   string
}

func (c *Config) Validate() error {
	var validateErr error
	if c.AzureSubscriptionId == "" {
		validateErr = multierr.Append(validateErr, fmt.Errorf("%s is required", AzureSubscriptionIdEnvKey))
	}
	if c.AzureTenantId == "" {
		validateErr = multierr.Append(validateErr, fmt.Errorf("%s is required", AzureTenantIdEnvKey))
	}
	if c.AzureClientId == "" {
		validateErr = multierr.Append(validateErr, fmt.Errorf("%s is required", AzureClientIdEnvKey))
	}
	if c.AzureClientSecret == "" {
		validateErr = multierr.Append(validateErr, fmt.Errorf("%s is required", AzureClientSecretEnvKey))
	}
	return validateErr
}

type AzureCostPerService struct {
	ServiceName string
	Cost        float64
}

var GlobalConfig Config

func main() {
	if err := command(); err != nil {
		fmt.Fprintf(os.Stdout, "%+v\n", err)
		os.Exit(1)
	}
}

func command() error {
	if err := initGlobalConifg(); err != nil {
		return err
	}
	beginningOfMonth := now.BeginningOfMonth()
	endOfMonth := now.EndOfMonth()

	cmClient := costmanagement.NewQueryClient(Global.AzureSubscriptionId)

	authorizer, err := auth.NewAuthorizerFromEnvironment()
	if err != nil {
		return err
	}
	cmClient.Authorizer = authorizer

	res, err := cmClient.Usage(context.Background(), "subscriptions/"+Global.AzureSubscriptionId, costmanagement.QueryDefinition{
		Type:      String("ActualCost"),
		Timeframe: "Custom",
		TimePeriod: &costmanagement.QueryTimePeriod{
			From: &date.Time{Time: beginningOfMonth},
			To:   &date.Time{Time: endOfMonth},
		},
		Dataset: &costmanagement.QueryDataset{
			Aggregation: map[string]*costmanagement.QueryAggregation{
				"totalCost": {
					Name:     String("PreTaxCost"),
					Function: String("Sum"),
				},
			},
			Grouping: &[]costmanagement.QueryGrouping{
				{
					Type: costmanagement.QueryColumnTypeDimension,
					Name: String("ServiceName"),
				},
			},
		},
	})

	if err != nil {
		return err
	}

	var costs []AzureCostPerService

	for _, row := range *res.Rows {
		costPerService := AzureCostPerService{}
		for n, val := range row {
			switch n {
			case 0: // ServiceName
				if cost, ok := val.(float64); ok {
					costPerService.Cost = cost
				}
			case 1: // PreTaxCost
				if serviceName, ok := val.(string); ok {
					costPerService.ServiceName = serviceName
				}
			case 2: // Currency (ignore)
			}
		}
		costs = append(costs, costPerService)
	}

	data, total := AzureCostPerServiceToTableWriter(costs)
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"ServiceName", "Cost"})
	table.SetFooter([]string{"Total", fmt.Sprintf("%.3f", total)})
	table.AppendBulk(data)
	table.Render()
	return nil
}

func initGlobalConifg() error {
	config := &Config{
		AzureSubscriptionId: os.Getenv(AzureSubscriptionIdEnvKey),
		AzureTenantId:       os.Getenv(AzureTenantIdEnvKey),
		AzureClientId:       os.Getenv(AzureClientIdEnvKey),
		AzureClientSecret:   os.Getenv(AzureClientSecretEnvKey),
	}
	if err := config.Validate(); err != nil {
		return err
	}
	Global = config
	return nil
}

func String(s string) *string {
	return &s
}

func AzureCostPerServiceToTableWriter(services []AzureCostPerService) ([][]string, float64) {
	total := float64(0)
	var data [][]string
	for _, service := range services {
		total += service.Cost
		data = append(data, []string{service.ServiceName, fmt.Sprintf("%.3f", service.Cost)})
	}
	return data, total
}
