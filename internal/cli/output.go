package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/mr-chelyshkin/limanix/internal/vm"
)

func writeJSON[T any](writer io.Writer, entries []T) error {
	if entries == nil {
		entries = []T{}
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(entries)
}

func writeInstances(output io.Writer, entries []vm.Info) error {
	if len(entries) == 0 {
		_, err := fmt.Fprintln(output, "No VMs managed by Limanix.")
		return err
	}
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "NAME\tSTATUS\tSTATE\tADDRESS"); err != nil {
		return err
	}
	for _, entry := range entries {
		backendStatus, operationStatus, address := "Missing", "corrupt", entry.Address
		if entry.BackendStatus != nil {
			backendStatus = string(*entry.BackendStatus)
		}
		if entry.OperationStatus != nil {
			operationStatus = string(*entry.OperationStatus)
		}
		if address == "" {
			address = "-"
		}
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", entry.Name, backendStatus, operationStatus, address); err != nil {
			return err
		}
		if entry.Error != nil {
			if _, err := fmt.Fprintf(writer, "  %s\n", *entry.Error); err != nil {
				return err
			}
		}
	}
	return writer.Flush()
}
