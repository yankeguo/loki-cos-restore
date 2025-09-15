package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	stdlog "log"
	"os"
	"reflect"
	"time"

	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/log"
	"github.com/grafana/loki/v3/pkg/logql/syntax"
	"github.com/grafana/loki/v3/pkg/loki"
	"github.com/grafana/loki/v3/pkg/querier/plan"
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
	var err error
	defer func() {
		if err == nil {
			return
		}
		stdlog.Println("exited with error:", err.Error())
		os.Exit(1)
	}()
	defer rg.Guard(&err)

	var (
		envUserID  = env("RESTORE_USER_ID", "fake")
		envTimeBeg = env("RESTORE_TIME_BEG", "")
		envTimeEnd = env("RESTORE_TIME_END", "")
		envQuery   = env("RESTORE_QUERY", "")
	)

	if envTimeBeg == "" {
		err = errors.New("missing -time-beg")
		return
	}

	if envTimeEnd == "" {
		err = errors.New("missing -time-end")
		return
	}

	timeBeg := rg.Must(time.Parse(time.RFC3339, envTimeBeg))

	timeEnd := rg.Must(time.Parse(time.RFC3339, envTimeEnd))

	if envQuery == "" {
		err = errors.New("missing -query")
		return
	}

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

	ins := rg.Must(loki.New(config.Config))

	expr := rg.Must(syntax.ParseExpr(envQuery))

	matchers := rg.Must(syntax.ParseMatchers(envQuery, true))

	predicate := chunk.NewPredicate(matchers, &plan.QueryPlan{
		AST: expr,
	})

	chunksGroup, _ := rg.Must2(ins.Store.GetChunks(
		context.Background(),
		envUserID,
		model.TimeFromUnix(timeBeg.Unix()),
		model.TimeFromUnix(timeEnd.Unix()),
		predicate,
		nil,
	))

	for i, chunks := range chunksGroup {
		for j, chunk := range chunks {
			stdlog.Printf(">>>>>>>>>> %d/%d\n%s", i, j, rg.Must(json.MarshalIndent(chunk, "", "  ")))
		}
	}
}
