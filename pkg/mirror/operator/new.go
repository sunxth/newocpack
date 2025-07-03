package operator

import (
	"ocpack/pkg/mirror/api/v2alpha1"
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
) CollectorInterface {
	return &LocalStorageCollector{OperatorCollector{Log: log, LogsDir: logsDir, Config: config, Opts: opts, Mirror: mirror, Manifest: manifest, LocalStorageFQDN: opts.LocalStorageFQDN, ctlgHandler: catalogHandler{Log: log}}}
}

func NewWithFilter(log clog.PluggableLoggerInterface,
	logsDir string,
	config v2alpha1.ImageSetConfiguration,
	opts mirror.CopyOptions,
	mirror mirror.MirrorInterface,
	manifest manifest.ManifestInterface,
) CollectorInterface {
	return &FilterCollector{OperatorCollector{Log: log, LogsDir: logsDir, Config: config, Opts: opts, Mirror: mirror, Manifest: manifest, LocalStorageFQDN: opts.LocalStorageFQDN, ctlgHandler: catalogHandler{Log: log}}}
}
