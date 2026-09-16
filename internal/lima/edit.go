package lima

import (
	"bytes"
	"context"
	"os"
	"path/filepath"

	"github.com/lima-vm/lima/v2/pkg/driverutil"
	"github.com/lima-vm/lima/v2/pkg/limatype"
	"github.com/lima-vm/lima/v2/pkg/limatype/filenames"
	"github.com/lima-vm/lima/v2/pkg/limayaml"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
)

// Edit follows Lima's native edit validation before replacing its persistent
// YAML. Driver defaults and constraints are checked against the actual instance,
// and the existing configuration stays intact when validation fails.
func (client *Client) Edit(ctx context.Context, name, path string) (failure error) {
	defer func() {
		failure = operationError(ctx, "edit", failure)
	}()

	inst, err := client.inspectInstance(ctx, name)
	if err != nil {
		return err
	}

	if inst.Status == limatype.StatusRunning {
		return ErrRunningEdit
	}

	if err := inspectionErrors(inst); err != nil {
		return err
	}

	yamlPath := filepath.Join(inst.Dir, filenames.LimaYAML)
	previous, err := os.ReadFile(yamlPath)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if bytes.Equal(data, previous) {
		return nil
	}

	if err := client.validateEdit(ctx, inst, yamlPath, data, previous); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	return filesystem.WriteFileAtomic(yamlPath, data, 0o600)
}

// validateEdit checks defaults, driver constraints and changes against the
// existing Lima configuration. The on-disk YAML remains untouched throughout.
func (client *Client) validateEdit(ctx context.Context, inst *limatype.Instance, yamlPath string, data, previous []byte) error {
	yaml, err := limayaml.Load(ctx, data, yamlPath)
	if err != nil {
		return err
	}

	if err := driverutil.ResolveVMType(yaml); err != nil {
		return err
	}

	if err := limayaml.Validate(yaml, false); err != nil {
		return err
	}

	inst.Config = yaml
	if err := client.native.validateDriver(ctx, inst); err != nil {
		return err
	}

	if err := limayaml.Validate(inst.Config, true); err != nil {
		return err
	}

	if err := limayaml.ValidateAgainstLatestConfig(ctx, data, previous); err != nil {
		return err
	}

	return nil
}
