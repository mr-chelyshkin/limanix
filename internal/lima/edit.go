package lima

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/lima-vm/lima/v2/pkg/driver/external/server"
	"github.com/lima-vm/lima/v2/pkg/driverutil"
	"github.com/lima-vm/lima/v2/pkg/limatype"
	"github.com/lima-vm/lima/v2/pkg/limatype/filenames"
	"github.com/lima-vm/lima/v2/pkg/limayaml"
	"github.com/mr-chelyshkin/limanix/internal/filesystem"
)

func validateConfiguredDriver(ctx context.Context, inst *limatype.Instance) error {
	limaDriver, err := driverutil.CreateConfiguredDriver(ctx, inst, 0)
	if err != nil {
		return err
	}
	defer server.Stop(inst.Dir, true)
	return limaDriver.Validate(ctx)
}

// Edit follows Lima's native edit validation before replacing its persistent
// YAML. Driver defaults and constraints are checked against the actual instance,
// and the existing configuration stays intact when validation fails.
func (client *Client) Edit(ctx context.Context, name, path string) error {
	inst, err := client.inspectInstance(ctx, name)
	if err != nil {
		return &Error{Operation: "edit", Err: err}
	}
	if inst.Status == limatype.StatusRunning {
		return &Error{Operation: "edit", Err: errors.New("cannot edit a running instance")}
	}
	if err := inspectionErrors(inst); err != nil {
		return &Error{Operation: "edit", Err: err}
	}
	yamlPath := filepath.Join(inst.Dir, filenames.LimaYAML)
	previous, err := os.ReadFile(yamlPath)
	if err != nil {
		return &Error{Operation: "edit", Err: err}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return &Error{Operation: "edit", Err: err}
	}
	if bytes.Equal(data, previous) {
		return nil
	}
	yaml, err := limayaml.Load(ctx, data, yamlPath)
	if err != nil {
		return &Error{Operation: "edit", Err: err}
	}
	if err := driverutil.ResolveVMType(yaml); err != nil {
		return &Error{Operation: "edit", Err: err}
	}
	if err := limayaml.Validate(yaml, false); err != nil {
		return &Error{Operation: "edit", Err: err}
	}
	inst.Config = yaml
	if err := client.validateDriver(ctx, inst); err != nil {
		return operationError(ctx, "edit", err)
	}
	if err := limayaml.Validate(inst.Config, true); err != nil {
		return &Error{Operation: "edit", Err: err}
	}
	if err := limayaml.ValidateAgainstLatestConfig(ctx, data, previous); err != nil {
		return &Error{Operation: "edit", Err: err}
	}
	if err := ctx.Err(); err != nil {
		return &Error{Operation: "edit", Err: err}
	}
	if err := filesystem.WriteFileAtomic(yamlPath, data, 0o600); err != nil {
		return &Error{Operation: "edit", Err: err}
	}
	return nil
}
