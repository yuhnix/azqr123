// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package vm

import (
	"strings"

	"github.com/Azure/azqr/internal/models"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v4"
)

// GetRecommendations - Returns the rules for the VirtualMachineScanner
func (a *VirtualMachineScanner) GetRecommendations() map[string]models.AzqrRecommendation {
	return map[string]models.AzqrRecommendation{
		"vm-003": {
			RecommendationID:   "vm-003",
			ResourceType:       "Microsoft.Compute/virtualMachines",
			Category:           models.CategoryHighAvailability,
			Recommendation:     "Virtual Machine should have a SLA",
			RecommendationType: models.TypeSLA,
			Impact:             models.ImpactHigh,
			Eval: func(target interface{}, scanContext *models.ScanContext) (bool, string) {
				v := target.(*armcompute.VirtualMachine)
				sla := "99.9%"
				hasScaleSet := v.Properties.VirtualMachineScaleSet != nil && v.Properties.VirtualMachineScaleSet.ID != nil
				hasZones := len(v.Zones) > 1

				if hasScaleSet && !hasZones {
					sla = "99.95%"
				} else if hasZones {
					sla = "99.99%"
				}
				return false, sla
			},
			LearnMoreUrl: "https://www.microsoft.com/licensing/docs/view/Service-Level-Agreements-SLA-for-Online-Services?lang=1",
		},
		"vm-006": {
			RecommendationID: "vm-006",
			ResourceType:     "Microsoft.Compute/virtualMachines",
			Category:         models.CategoryGovernance,
			Recommendation:   "Virtual Machine Name should comply with naming conventions",
			Impact:           models.ImpactLow,
			Eval: func(target interface{}, scanContext *models.ScanContext) (bool, string) {
				c := target.(*armcompute.VirtualMachine)
				caf := strings.HasPrefix(*c.Name, "vm")
				return !caf, ""
			},
			LearnMoreUrl: "https://learn.microsoft.com/en-us/azure/cloud-adoption-framework/ready/azure-best-practices/resource-abbreviations",
		},
		"vm-007": {
			RecommendationID: "vm-007",
			ResourceType:     "Microsoft.Compute/virtualMachines",
			Category:         models.CategoryGovernance,
			Recommendation:   "Virtual Machine should have tags",
			Impact:           models.ImpactLow,
			Eval: func(target interface{}, scanContext *models.ScanContext) (bool, string) {
				c := target.(*armcompute.VirtualMachine)
				return len(c.Tags) == 0, ""
			},
			LearnMoreUrl: "https://learn.microsoft.com/en-us/azure/azure-resource-manager/management/tag-resources?tabs=json",
		},
		"vm-008": {
			RecommendationID: "vm-008",
			ResourceType:     "Microsoft.Compute/virtualMachines",
			Category:         models.CategorySecurity,
			Recommendation:   "Virtual Machine should have disk encryption at host enabled",
			Impact:           models.ImpactHigh,
			Eval: func(target interface{}, scanContext *models.ScanContext) (bool, string) {
				v := target.(*armcompute.VirtualMachine)
				isDiskEncryptionEnabled := v.Properties.SecurityProfile != nil && v.Properties.SecurityProfile.EncryptionAtHost != nil && *v.Properties.SecurityProfile.EncryptionAtHost
				return !isDiskEncryptionEnabled, ""
			},
			LearnMoreUrl: "https://learn.microsoft.com/en-us/azure/security-center/security-center-disk-encryption",
		},
		// find disks with azure disk encryption set
		"vm-009": {
			RecommendationID: "vm-009",
			ResourceType:     "Microsoft.Compute/virtualMachines",
			Category:         models.CategorySecurity,
			Recommendation:   "Virtual Machine has Azure Disk Encryption (ADE) enabled",
			Impact:           models.ImpactHigh,
			Eval: func(target interface{}, scanContext *models.ScanContext) (bool, string) {
				v := target.(*armcompute.VirtualMachine)

				// Check if we have VM details for this VM
				if scanContext.VMDetails != nil {
					if vmDetails, exists := scanContext.VMDetails[*v.ID]; exists {
						// Return the ADE status from VMResult
						return !vmDetails.ADEEnabled, vmDetails.DiskSSEType
					}
				}

				// If we couldn't determine, fail the check
				return true, "Unable to determine ADE status"
			},
			LearnMoreUrl: "https://learn.microsoft.com/en-us/azure/virtual-machines/azure-disk-encryption-overview",
		},
	}
}
