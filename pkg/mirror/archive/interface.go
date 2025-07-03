package archive

import (
	"context"

	"ocpack/pkg/mirror/api/v2alpha1"
)

type BlobsGatherer interface {
	GatherBlobs(ctx context.Context, imgRef string) (map[string]struct{}, error)
}

type Archiver interface {
	BuildArchive(ctx context.Context, collectedImages []v2alpha1.CopyImageSchema) error
}

type UnArchiver interface {
	Unarchive() error
}

type archiveAdder interface {
	addFile(pathToFile string, pathInTar string) error
	addAllFolder(folderToAdd string, relativeTo string) error
	close() error
}
