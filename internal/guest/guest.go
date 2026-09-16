package guest

import "context"

// Client is the management connection needed for guest operations.
// Run must support concurrent address probes for different VM instances.
type Client interface {
	Run(context.Context, string, []string, bool) (string, error)
	Start(context.Context, string) error
	Stop(context.Context, string) error
	Shell(context.Context, string, []string) (int, error)
}

// Guest uses a management connection to provision and access the regular guest account.
type Guest struct {
	client Client
}

// New binds guest operations to the supplied management connection.
func New(client Client) *Guest {
	return &Guest{
		client: client,
	}
}
