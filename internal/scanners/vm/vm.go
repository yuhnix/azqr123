// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package vm

import (
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
	config    *models.ScannerConfig
	client    *armcompute.VirtualMachinesClient
	VMResults []VMResult // Store VM details for Excel export
}

// VMResult represents detailed VM properties for Excel export
type VMResult struct {
	SubscriptionID   string
	ResourceName     string
	ResourceGroup    string
	Location         string
	OsType           string
	ImagePublisher   string
	ImageOffer       string
	ImagePlan        string
	SKU              string
	IsSQLVM          bool
	PublicIP         string
	AvailabilitySet  bool
	EncryptionAtHost bool
	ADEEnabled       bool
	ADEProvisioning  string
	DiskSSEType      string
}

// Init - Initializes the VirtualMachineScanner
func (c *VirtualMachineScanner) Init(config *models.ScannerConfig) error {
	c.config = config
	c.VMResults = make([]VMResult, 0) // Initialize the slice
	var err error
	c.client, err = armcompute.NewVirtualMachinesClient(config.SubscriptionID, config.Cred, config.ClientOptions)
	return err
}

// Scan - Scans all Virtual Machines in a Resource Group
func (c *VirtualMachineScanner) Scan(scanContext *models.ScanContext) ([]models.AzqrServiceResult, error) {
	models.LogSubscriptionScan(c.config.SubscriptionID, c.ResourceTypes()[0])

	vms, err := c.list()
	if err != nil {
		return nil, err
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

		// Extract detailed VM properties for Excel export
		vmResult := c.extractVMDetails(vm)
		c.VMResults = append(c.VMResults, vmResult)
	}

	return results, nil
}

// extractVMDetails extracts detailed VM properties
func (c *VirtualMachineScanner) extractVMDetails(vm *armcompute.VirtualMachine) VMResult {
	result := VMResult{
		SubscriptionID: c.config.SubscriptionID,
		ResourceName:   to.String(vm.Name),
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

		// Check for SQL VM (based on image publisher/offer)
		if result.ImagePublisher == "MicrosoftSQLServer" {
			result.IsSQLVM = true
		}

		// Extract public IP information (requires network interface lookup)
		// Note: This would require additional API calls to get network interface details
		// For now, leaving empty - can be enhanced later
		result.PublicIP = ""

		// Extract Azure Disk Encryption (ADE) information
		// This would require checking disk encryption settings
		// For now, setting defaults - can be enhanced later
		result.ADEEnabled = false
		result.ADEProvisioning = ""
		result.DiskSSEType = ""
	}

	// Extract plan information from VM plan (not properties)
	if vm.Plan != nil {
		result.ImagePlan = to.String(vm.Plan.Name)
	}

	return result
}

// list retrieves all virtual machines from the subscription
func (c *VirtualMachineScanner) list() ([]*armcompute.VirtualMachine, error) {
	pager := c.client.NewListAllPager(nil)

	vms := make([]*armcompute.VirtualMachine, 0)
	for pager.More() {
		// Wait for a token from the burstLimiter channel before making the request
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
func (c *VirtualMachineScanner) GetVMResults() []VMResult {
	return c.VMResults
}
