// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package vm

import (
	"strings"

	"github.com/Azure/azqr/internal/models"
	"github.com/Azure/azqr/internal/throttling"
	"github.com/Azure/azqr/internal/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v4"
)

func init() {
	models.ScannerList["vm"] = []models.IAzureScanner{&VirtualMachineScanner{}}
}

// VirtualMachineScanner - Scanner for Virtual Machines
type VirtualMachineScanner struct {
	config      *models.ScannerConfig
	client      *armcompute.VirtualMachinesClient
	disksClient *armcompute.DisksClient
	VMResults   []models.VMResult // Nu uit models package
}

// Init - Initializes the VirtualMachineScanner
func (c *VirtualMachineScanner) Init(config *models.ScannerConfig) error {
	c.config = config
	c.VMResults = make([]models.VMResult, 0)

	var err error
	c.client, err = armcompute.NewVirtualMachinesClient(config.SubscriptionID, config.Cred, config.ClientOptions)
	if err != nil {
		return err
	}

	c.disksClient, err = armcompute.NewDisksClient(config.SubscriptionID, config.Cred, config.ClientOptions)
	if err != nil {
		return err
	}

	return nil
}

// Scan - Scans all Virtual Machines in a Resource Group
func (c *VirtualMachineScanner) Scan(scanContext *models.ScanContext) ([]models.AzqrServiceResult, error) {
	models.LogSubscriptionScan(c.config.SubscriptionID, c.ResourceTypes()[0])

	vms, err := c.list()
	if err != nil {
		return nil, err
	}

	// Collect detailed VM info (including disk encryption) for all VMs
	vmDetailsMap := make(map[string]*models.VMResult)
	for _, vm := range vms {
		vmDetails := c.extractVMDetails(vm)
		vmDetailsMap[*vm.ID] = &vmDetails
		c.VMResults = append(c.VMResults, vmDetails)
	}

	// Add VM details to scan context so recommendations can access it
	if scanContext.VMDetails == nil {
		scanContext.VMDetails = make(map[string]*models.VMResult)
	}
	for k, v := range vmDetailsMap {
		scanContext.VMDetails[k] = v
	}

	engine := models.RecommendationEngine{}
	rules := c.GetRecommendations()
	results := []models.AzqrServiceResult{}

	for _, vm := range vms {
		// Evaluate recommendations
		rr := engine.EvaluateRecommendations(rules, vm, scanContext)

		// Create standard azqr result
		results = append(results, models.AzqrServiceResult{
			SubscriptionID:   c.config.SubscriptionID,
			SubscriptionName: c.config.SubscriptionName,
			ResourceGroup:    models.GetResourceGroupFromResourceID(*vm.ID),
			ServiceName:      *vm.Name,
			Type:             *vm.Type,
			Location:         *vm.Location,
			Recommendations:  rr,
		})
	}

	return results, nil
}

// extractVMDetails extracts detailed VM properties including disk encryption
func (c *VirtualMachineScanner) extractVMDetails(vm *armcompute.VirtualMachine) models.VMResult {
	result := models.VMResult{
		SubscriptionID: c.config.SubscriptionID,
		ResourceName:   to.String(vm.Name),
		ResourceID:     to.String(vm.ID),
		ResourceGroup:  models.GetResourceGroupFromResourceID(*vm.ID),
		Location:       to.String(vm.Location),
	}

	if vm.Properties != nil {
		// Extract OS type
		if vm.Properties.StorageProfile != nil && vm.Properties.StorageProfile.OSDisk != nil && vm.Properties.StorageProfile.OSDisk.OSType != nil {
			result.OsType = string(*vm.Properties.StorageProfile.OSDisk.OSType)
		}

		// Extract image information
		if vm.Properties.StorageProfile != nil && vm.Properties.StorageProfile.ImageReference != nil {
			imageRef := vm.Properties.StorageProfile.ImageReference
			result.ImagePublisher = to.String(imageRef.Publisher)
			result.ImageOffer = to.String(imageRef.Offer)
		}

		// Extract VM size/SKU
		if vm.Properties.HardwareProfile != nil && vm.Properties.HardwareProfile.VMSize != nil {
			result.SKU = string(*vm.Properties.HardwareProfile.VMSize)
		}

		// Check for availability set
		result.AvailabilitySet = vm.Properties.AvailabilitySet != nil

		// Check encryption at host
		if vm.Properties.SecurityProfile != nil && vm.Properties.SecurityProfile.EncryptionAtHost != nil {
			result.EncryptionAtHost = *vm.Properties.SecurityProfile.EncryptionAtHost
		}

		// Check for SQL VM
		if result.ImagePublisher == "MicrosoftSQLServer" {
			result.IsSQLVM = true
		}

		// Extract disk encryption information
		if vm.Properties.StorageProfile != nil &&
			vm.Properties.StorageProfile.OSDisk != nil &&
			vm.Properties.StorageProfile.OSDisk.ManagedDisk != nil &&
			vm.Properties.StorageProfile.OSDisk.ManagedDisk.ID != nil {

			diskID := *vm.Properties.StorageProfile.OSDisk.ManagedDisk.ID
			parts := strings.Split(diskID, "/")
			diskName := parts[len(parts)-1]

			// Extract resource group from disk ID
			rg := ""
			for i, part := range parts {
				if strings.EqualFold(part, "resourceGroups") && i+1 < len(parts) {
					rg = parts[i+1]
					break
				}
			}

			if rg != "" {
				<-throttling.ARMLimiter
				disk, err := c.disksClient.Get(c.config.Ctx, rg, diskName, nil)
				if err == nil && disk.Properties != nil {
					// Check for SSE encryption type
					if disk.Properties.Encryption != nil && disk.Properties.Encryption.Type != nil {
						result.DiskSSEType = string(*disk.Properties.Encryption.Type)
					}

					// Check for ADE (Azure Disk Encryption)
					if disk.Properties.EncryptionSettingsCollection != nil &&
						disk.Properties.EncryptionSettingsCollection.Enabled != nil {
						result.ADEEnabled = *disk.Properties.EncryptionSettingsCollection.Enabled

						// Get provisioning state if available
						if disk.Properties.ProvisioningState != nil {
							result.ADEProvisioning = *disk.Properties.ProvisioningState
						}
					}
				}
			}
		}
	}

	// Extract plan information from VM plan (not properties)
	if vm.Plan != nil {
		result.ImagePlan = to.String(vm.Plan.Name)
	}

	// Public IP would require network interface lookup - leaving empty for now
	result.PublicIP = ""

	return result
}

// list retrieves all virtual machines from the subscription
func (c *VirtualMachineScanner) list() ([]*armcompute.VirtualMachine, error) {
	pager := c.client.NewListAllPager(nil)

	vms := make([]*armcompute.VirtualMachine, 0)
	for pager.More() {
		<-throttling.ARMLimiter
		resp, err := pager.NextPage(c.config.Ctx)
		if err != nil {
			return nil, err
		}
		vms = append(vms, resp.Value...)
	}
	return vms, nil
}

// ResourceTypes returns the resource types scanned by this scanner
func (c *VirtualMachineScanner) ResourceTypes() []string {
	return []string{"Microsoft.Compute/virtualMachines"}
}

// GetVMResults returns the collected VM details for Excel export
func (c *VirtualMachineScanner) GetVMResults() []models.VMResult {
	return c.VMResults
}
