package additional

import (
	"ocpack/pkg/mirror/api/v2alpha1"
	clog "ocpack/pkg/mirror/log"
	"ocpack/pkg/mirror/manifest"
	"ocpack/pkg/mirror/mirror"
)

func New(log clog.PluggableLoggerInterface,
	config v2alpha1.ImageSetConfiguration,
	opts mirror.CopyOptions,
	mirror mirror.MirrorInterface,
	manifest manifest.ManifestInterface,
) CollectorInterface {
	return &LocalStorageCollector{Log: log, Config: config, Opts: opts, Mirror: mirror, Manifest: manifest, LocalStorageFQDN: opts.LocalStorageFQDN}
}
