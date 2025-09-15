package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	stdlog "log"
	"net/http"
	"net/url"
	"os"
	"path"
	"reflect"
	"strconv"
	"time"

	"github.com/tencentyun/cos-go-sdk-v5"

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
		envDays    = env("RESTORE_DAYS", "3")
		envTier    = env("RESTORE_TIER", "Standard")
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

	days, _ := strconv.Atoi(envDays)

	if days < 1 {
		days = 1
	}

	if envTier != "Standard" && envTier != "Bulk" {
		envTier = "Standard"
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

	awsConfig := config.StorageConfig.AWSStorageConfig

	cosClient := cos.NewClient(&cos.BaseURL{
		BucketURL:  rg.Must(url.Parse("https://" + awsConfig.BucketNames + "." + awsConfig.Endpoint)),
		ServiceURL: rg.Must(url.Parse("https://" + awsConfig.Endpoint)),
	}, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  awsConfig.AccessKeyID,
			SecretKey: awsConfig.SecretAccessKey.String(),
		},
	})
	cosClient.Conf.RetryOpt.Interval = time.Second / 2

	restoreCos := func(filename string) {
		if _, err := cosClient.Object.PostRestore(context.Background(), filename, &cos.ObjectRestoreOptions{
			Days: days,
			Tier: &cos.CASJobParameters{
				Tier: envTier,
			},
		}); err != nil {
			stdlog.Printf("%s: %s", filename, err.Error())
		} else {
			stdlog.Printf("%s: restoring", filename)
		}
	}

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
			restoreCos(filename)
		}
	}
}
