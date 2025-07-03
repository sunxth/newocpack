package batch

import (
	"context"

	"ocpack/pkg/mirror/api/v2alpha1"
	"ocpack/pkg/mirror/mirror"
)

type BatchInterface interface {
	Worker(ctx context.Context, collectorSchema v2alpha1.CollectorSchema, opts mirror.CopyOptions) (v2alpha1.CollectorSchema, error)
}
