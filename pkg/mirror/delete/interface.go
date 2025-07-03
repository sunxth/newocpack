package delete

import (
	"context"

	"ocpack/pkg/mirror/api/v2alpha1"
)

type DeleteInterface interface {
	WriteDeleteMetaData(context.Context, []v2alpha1.CopyImageSchema) error
	ReadDeleteMetaData() (v2alpha1.DeleteImageList, error)
	DeleteRegistryImages(images v2alpha1.DeleteImageList) error
}
