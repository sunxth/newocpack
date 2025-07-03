package clusterresources

import (
	"ocpack/pkg/mirror/api/v2alpha1"
)

type GeneratorInterface interface {
	IDMS_ITMSGenerator(allRelatedImages []v2alpha1.CopyImageSchema, forceRepositoryScope bool) error
	UpdateServiceGenerator(graphImage, releaseImage string) error
	CatalogSourceGenerator(allRelatedImages []v2alpha1.CopyImageSchema) error
	GenerateSignatureConfigMap(allRelatedImages []v2alpha1.CopyImageSchema) error
	ClusterCatalogGenerator(allRelatedImages []v2alpha1.CopyImageSchema) error
}
