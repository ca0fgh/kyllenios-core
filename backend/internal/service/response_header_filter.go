package service

import (
	"github.com/ca0fgh/kyllenios-core/internal/config"
	"github.com/ca0fgh/kyllenios-core/internal/util/responseheaders"
)

func compileResponseHeaderFilter(cfg *config.Config) *responseheaders.CompiledHeaderFilter {
	if cfg == nil {
		return nil
	}
	return responseheaders.CompileHeaderFilter(cfg.Security.ResponseHeaders)
}
