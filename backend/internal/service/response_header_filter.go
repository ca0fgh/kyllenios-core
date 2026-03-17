package service

import (
	"github.com/ca0fgh/hermes-proxy/internal/config"
	"github.com/ca0fgh/hermes-proxy/internal/util/responseheaders"
)

func compileResponseHeaderFilter(cfg *config.Config) *responseheaders.CompiledHeaderFilter {
	if cfg == nil {
		return nil
	}
	return responseheaders.CompileHeaderFilter(cfg.Security.ResponseHeaders)
}
