package ready

import "context"

//go:generate mockgen -source=contract.go -destination=mocks_test.go -package=ready_test

type database interface {
	Ping(ctx context.Context) error
}
