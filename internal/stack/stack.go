package stack

import "context"

// Driver controls the lifecycle of a packet processing backend.
type Driver interface {
	Run(ctx context.Context) error
}
