package additional

import (
	"context"

	"ocpack/pkg/mirror/api/v2alpha1"
)

type CollectorInterface interface {
	AdditionalImagesCollector(ctx context.Context) ([]v2alpha1.CopyImageSchema, error)
}
