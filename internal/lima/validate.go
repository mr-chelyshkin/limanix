package lima

import (
	"context"
	"os"
	"path/filepath"

	"github.com/lima-vm/lima/v2/pkg/limayaml"
)

// Validate loads defaults and validates through Lima's native schema.
func (client *Client) Validate(ctx context.Context, path string) (failure error) {
	defer func() {
		failure = operationError(ctx, "validate", failure)
	}()

	if err := ctx.Err(); err != nil {
		return err
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(absolute)
	if err != nil {
		return err
	}

	yaml, err := limayaml.Load(ctx, data, absolute)
	if err != nil {
		return err
	}

	if err = limayaml.Validate(yaml, false); err != nil {
		return err
	}

	return ctx.Err()
}
