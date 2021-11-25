package confgo

import (
	"github.com/douyu/juno/internal/pkg/invoker"
	"github.com/douyu/juno/pkg/cfg"
)

var (
	// Cmc ...
	Cmc *cmc
	// ConfuSrv ...
	ConfuSrv *confu
)

// Init ...
func Init() {
	Cmc = &cmc{}
	ConfuSrv = &confu{
		DB:      invoker.JunoMysql,
		GitPath: cfg.Cfg.Configure.GitPath,
		GitRepo: cfg.Cfg.Configure.GitRepo,
	}
}
