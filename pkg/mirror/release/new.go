package release

import (
	"ocpack/pkg/mirror/api/v2alpha1"
	"ocpack/pkg/mirror/imagebuilder"
	clog "ocpack/pkg/mirror/log"
	"ocpack/pkg/mirror/manifest"
	"ocpack/pkg/mirror/mirror"
)

func New(log clog.PluggableLoggerInterface,
	logsDir string,
	config v2alpha1.ImageSetConfiguration,
	opts mirror.CopyOptions,
	mirror mirror.MirrorInterface,
	manifest manifest.ManifestInterface,
	cincinnati CincinnatiInterface,
	imageBuilder imagebuilder.ImageBuilderInterface,
) CollectorInterface {
	return &LocalStorageCollector{Log: log, LogsDir: logsDir, Config: config, Opts: opts, Mirror: mirror, Manifest: manifest, Cincinnati: cincinnati, LocalStorageFQDN: opts.LocalStorageFQDN, ImageBuilder: imageBuilder}
}
