package helm

import (
	"path/filepath"

	"ocpack/pkg/mirror/api/v2alpha1"
	clog "ocpack/pkg/mirror/log"
	"ocpack/pkg/mirror/mirror"
)

func New(log clog.PluggableLoggerInterface,
	config v2alpha1.ImageSetConfiguration,
	opts mirror.CopyOptions,
	indexDownloader indexDownloader,
	chartDownloader chartDownloader,
	httpClient webClient,
) CollectorInterface {
	lsc = &LocalStorageCollector{Log: log, Config: config, Opts: opts, Helm: NewHelmOptions(opts.SrcImage.TlsVerify)}
	lsc.Log.Debug("helm.New opts.SrcImage.TlsVerify %t", opts.SrcImage.TlsVerify)

	if !opts.IsDiskToMirror() {
		wClient = httpClient

		cleanup, file, _ := createTempFile(filepath.Join(lsc.Opts.Global.WorkingDir, helmDir))
		lsc.Helm.settings.RepositoryConfig = file
		lsc.cleanup = cleanup

		lsc.Downloaders.indexDownloader = indexDownloader

		if chartDownloader == nil {
			lsc.Downloaders.chartDownloader = GetDefaultChartDownloader()

		} else {
			lsc.Downloaders.chartDownloader = chartDownloader
		}
	}

	return lsc
}
