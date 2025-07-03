package batch

import (
	"ocpack/pkg/mirror/api/v2alpha1"
	clog "ocpack/pkg/mirror/log"
)

type ProgressStruct struct {
	Log clog.PluggableLoggerInterface
}

type StringMap map[string]string
type onlyKeyMap map[string]struct{}

type mirrorErrorSchema struct {
	image     v2alpha1.CopyImageSchema
	err       error
	operators map[string]struct{}
	bundles   StringMap
}

func (e mirrorErrorSchema) Error() string {
	return e.err.Error()
}

func (s onlyKeyMap) Has(key string) bool {
	_, ok := s[key]
	return ok
}
