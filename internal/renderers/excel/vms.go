// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package excel

import (
	_ "image/png"

	"github.com/Azure/azqr/internal/renderers"
	"github.com/rs/zerolog/log"
	"github.com/xuri/excelize/v2"
)

// renderVms renders the Virtual Machines sheet in the Excel report
func renderVms(f *excelize.File, data *renderers.ReportData) {
	if len(data.VirtualMachines) == 0 {
		log.Info().Msg("Skipping Virtual Machines. No data to render")
		return
	}

	_, err := f.NewSheet("Virtual Machines")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create Virtual Machines sheet")
	}

	records := data.VirtualMachinesTable()
	headers := records[0]
	createFirstRow(f, "Virtual Machines", headers)

	records = records[1:]
	currentRow := 4
	for _, row := range records {
		currentRow++
		cell, err := excelize.CoordinatesToCellName(1, currentRow)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to get cell")
		}
		err = f.SetSheetRow("Virtual Machines", cell, &row)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to set row")
		}
	}

	configureSheet(f, "Virtual Machines", headers, currentRow)
}
