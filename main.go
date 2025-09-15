package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	stdlog "log"
	"os"
	"path"
	"reflect"
	"time"

	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/log"
	"github.com/grafana/loki/v3/pkg/logql/syntax"
	"github.com/grafana/loki/v3/pkg/loki"
	loki_runtime "github.com/grafana/loki/v3/pkg/runtime"
	"github.com/grafana/loki/v3/pkg/storage/chunk"
	_ "github.com/grafana/loki/v3/pkg/util/build"
	"github.com/grafana/loki/v3/pkg/util/cfg"
	util_log "github.com/grafana/loki/v3/pkg/util/log"
	"github.com/grafana/loki/v3/pkg/validation"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/yankeguo/rg"
)

func env(key string, defaultValue string) string {
	val := os.Getenv(key)
	if val == "" {
		val = defaultValue
	}
	if val == "" {
		rg.Must0(errors.New("missing environment variable: " + key))
	}
	return val
}

func main() {
	var (
		envUserID  = env("RESTORE_USER_ID", "fake")
		envTimeBeg = env("RESTORE_TIME_BEG", "")
		envTimeEnd = env("RESTORE_TIME_END", "")
		envQuery   = env("RESTORE_QUERY", "")
	)

	if envTimeBeg == "" {
		rg.Must0(errors.New("missing -time-beg"))
		return
	}

	if envTimeEnd == "" {
		rg.Must0(errors.New("missing -time-end"))
		return
	}

	timeBeg := rg.Must(time.Parse(time.RFC3339, envTimeBeg))

	timeEnd := rg.Must(time.Parse(time.RFC3339, envTimeEnd))

	if envQuery == "" {
		rg.Must0(errors.New("missing -query"))
		return
	}

	stdlog.Println("user_id:", envUserID, "time_beg:", envTimeBeg, "time_end:", envTimeEnd, "query:", envQuery)

	var config loki.ConfigWrapper
	rg.Must0(cfg.DynamicUnmarshal(&config, os.Args[1:], flag.CommandLine))
	rg.Must0(config.Validate())
	config.LimitsConfig.SetGlobalOTLPConfig(config.Distributor.OTLPConfig)
	validation.SetDefaultLimitsForYAMLUnmarshalling(config.LimitsConfig)
	loki_runtime.SetDefaultLimitsForYAMLUnmarshalling(config.OperationalConfig)
	if reflect.DeepEqual(&config.Server.LogLevel, &log.Level{}) {
		level.Error(util_log.Logger).Log("msg", "invalid log level")
	}
	serverCfg := &config.Server
	serverCfg.Log = util_log.InitLogger(serverCfg, prometheus.DefaultRegisterer, false)
	if config.InternalServer.Enable {
		config.InternalServer.Log = serverCfg.Log
	}

	stdlog.Println("configuration initialized")

	ins := rg.Must(loki.New(config.Config))

	stdlog.Println("loki instance created")

	ins.ModuleManager.InitModuleServices(loki.Store)

	stdlog.Println("loki instance store module initialized")

	matchers := rg.Must(syntax.ParseMatchers(envQuery, true))

	stdlog.Println("query matchers parsed")

	chunksGroup, _ := rg.Must2(ins.Store.GetChunks(
		context.Background(),
		envUserID,
		model.TimeFromUnix(timeBeg.Unix()),
		model.TimeFromUnix(timeEnd.Unix()),
		chunk.NewPredicate(matchers, nil),
		nil,
	))

	stdlog.Println("chunks groups found:", len(chunksGroup))

	for _, chunks := range chunksGroup {
		for _, chunk := range chunks {
			streamHash := fmt.Sprintf("%016x", chunk.ChunkRef.Fingerprint)
			chunkID := fmt.Sprintf("%x:%x:%x", int64(chunk.ChunkRef.From), int64(chunk.ChunkRef.Through), chunk.ChunkRef.Checksum)
			filename := path.Join(chunk.UserID, streamHash, chunkID)
			stdlog.Println(filename)
		}
	}
}
