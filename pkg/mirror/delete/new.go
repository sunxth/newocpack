package delete

import (
	"ocpack/pkg/mirror/api/v2alpha1"
	"ocpack/pkg/mirror/archive"
	"ocpack/pkg/mirror/batch"
	clog "ocpack/pkg/mirror/log"
	"ocpack/pkg/mirror/manifest"
	"ocpack/pkg/mirror/mirror"
	"ocpack/pkg/mirror/signature"
)

func New(log clog.PluggableLoggerInterface,
	opts mirror.CopyOptions,
	batch batch.BatchInterface,
	blobs archive.BlobsGatherer,
	config v2alpha1.ImageSetConfiguration,
	manifest manifest.ManifestInterface,
	localStorageDisk string,
	sigHandler signature.SignatureInterface,
) DeleteInterface {
	return &DeleteImages{
		Log:              log,
		Opts:             opts,
		Batch:            batch,
		Blobs:            blobs,
		Config:           config,
		Manifest:         manifest,
		LocalStorageDisk: localStorageDisk,
		LocalStorageFQDN: opts.LocalStorageFQDN,
		SigHandler:       sigHandler,
	}
}
