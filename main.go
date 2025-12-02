package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/tencentyun/cos-go-sdk-v5"

	"github.com/go-kit/log/level"
	glog "github.com/grafana/dskit/log"
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
	if val := os.Getenv(key); val != "" {
		return val
	}
	if defaultValue == "" {
		rg.Must0(errors.New("missing environment variable: " + key))
	}
	return defaultValue
}

func main() {
	ctx := context.Background()

	// Parse and validate environment variables
	envUserIDs := env("RESTORE_USER_ID", "fake")
	envTimeBeg := env("RESTORE_TIME_BEG", "")
	envTimeEnd := env("RESTORE_TIME_END", "")
	envQuery := env("RESTORE_QUERY", "")
	envDays := env("RESTORE_DAYS", "3")
	envTier := env("RESTORE_TIER", "Standard")

	// Parse user IDs (comma-separated)
	var userIDs []string
	for _, uid := range strings.Split(envUserIDs, ",") {
		uid = strings.TrimSpace(uid)
		if uid != "" {
			userIDs = append(userIDs, uid)
		}
	}
	if len(userIDs) == 0 {
		rg.Must0(errors.New("no valid user IDs provided"))
	}

	// Parse time ranges
	timeBeg := rg.Must(time.Parse(time.RFC3339, envTimeBeg))
	timeEnd := rg.Must(time.Parse(time.RFC3339, envTimeEnd))

	// Parse and validate days
	days, _ := strconv.Atoi(envDays)
	if days < 1 {
		days = 1
	}

	// Validate restore tier
	if envTier != "Standard" && envTier != "Bulk" {
		envTier = "Standard"
	}

	log.Println("user_ids:", userIDs, "time_beg:", envTimeBeg, "time_end:", envTimeEnd, "query:", envQuery)

	// Initialize Loki configuration
	var config loki.ConfigWrapper
	rg.Must0(cfg.DynamicUnmarshal(&config, os.Args[1:], flag.CommandLine))
	rg.Must0(config.Validate())

	// Configure logging
	config.LimitsConfig.SetGlobalOTLPConfig(config.Distributor.OTLPConfig)
	validation.SetDefaultLimitsForYAMLUnmarshalling(config.LimitsConfig)
	loki_runtime.SetDefaultLimitsForYAMLUnmarshalling(config.OperationalConfig)

	serverCfg := &config.Server
	serverCfg.Log = util_log.InitLogger(serverCfg, prometheus.DefaultRegisterer, false)
	if config.InternalServer.Enable {
		config.InternalServer.Log = serverCfg.Log
	}

	if reflect.DeepEqual(&config.Server.LogLevel, &glog.Level{}) {
		level.Error(util_log.Logger).Log("msg", "invalid log level")
	}

	log.Println("configuration initialized")

	// Initialize COS client
	awsConfig := config.StorageConfig.AWSStorageConfig
	bucketURL := rg.Must(url.Parse("https://" + awsConfig.BucketNames + "." + awsConfig.Endpoint))
	serviceURL := rg.Must(url.Parse("https://" + awsConfig.Endpoint))

	cosClient := cos.NewClient(&cos.BaseURL{
		BucketURL:  bucketURL,
		ServiceURL: serviceURL,
	}, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  awsConfig.AccessKeyID,
			SecretKey: awsConfig.SecretAccessKey.String(),
		},
	})
	cosClient.Conf.RetryOpt.Interval = time.Second / 2

	cosCheckFile := func(ctx context.Context, filename string) (size int64, needRestore bool) {
		res, err := cosClient.Object.Head(ctx, filename, &cos.ObjectHeadOptions{})
		if err != nil {
			return 0, false
		}

		size, _ = strconv.ParseInt(res.Header.Get("Content-Length"), 10, 64)
		storageClass := strings.ToUpper(res.Header.Get("x-cos-storage-class"))
		restoreStatus := strings.ToLower(res.Header.Get("x-cos-restore"))

		needRestore = strings.Contains(storageClass, "ARCHIVE") &&
			!strings.Contains(restoreStatus, "ongoing-request")

		return size, needRestore
	}

	cosRestoreFile := func(ctx context.Context, filename string) {
		if _, err := cosClient.Object.PostRestore(ctx, filename, &cos.ObjectRestoreOptions{
			Days: days,
			Tier: &cos.CASJobParameters{
				Tier: envTier,
			},
		}); err != nil {
			log.Printf("%s: %s", filename, err.Error())
		} else {
			log.Printf("%s: restoring", filename)
		}
	}

	// Initialize Loki instance
	ins := rg.Must(loki.New(config.Config))
	log.Println("loki instance created")

	ins.ModuleManager.InitModuleServices(loki.Store)
	log.Println("loki instance store module initialized")

	// Parse query matchers
	matchers := rg.Must(syntax.ParseMatchers(envQuery, true))
	log.Println("query matchers parsed")

	// Get chunks from Loki store for all user IDs
	var filenames []string
	for _, userID := range userIDs {
		chunksGroup, _ := rg.Must2(ins.Store.GetChunks(
			ctx,
			userID,
			model.TimeFromUnix(timeBeg.Unix()),
			model.TimeFromUnix(timeEnd.Unix()),
			chunk.NewPredicate(matchers, nil),
			nil,
		))
		log.Printf("user_id: %s, chunks groups found: %d", userID, len(chunksGroup))

		// Build filename list from chunks
		for _, chunks := range chunksGroup {
			for _, chunk := range chunks {
				filename := path.Join(
					chunk.UserID,
					fmt.Sprintf("%016x", chunk.ChunkRef.Fingerprint),
					fmt.Sprintf("%x:%x:%x", int64(chunk.ChunkRef.From), int64(chunk.ChunkRef.Through), chunk.ChunkRef.Checksum),
				)
				filenames = append(filenames, filename)
			}
		}
	}
	log.Println("found chunk files:", len(filenames))

	// Analyze files and calculate totals
	var totalSize, totalRestoreSize int64
	var filesToRestore []string

	for _, filename := range filenames {
		size, needRestore := cosCheckFile(ctx, filename)
		totalSize += size

		if needRestore {
			totalRestoreSize += size
			filesToRestore = append(filesToRestore, filename)
		}

		log.Printf("file: %s, size: %d bytes, need_restore: %v", filename, size, needRestore)
	}

	// Report summary
	log.Printf("total files: %d, total size: %d bytes", len(filenames), totalSize)
	log.Printf("files needing restore: %d, total restore size: %d bytes", len(filesToRestore), totalRestoreSize)

	// Restore files that need restoration
	for _, filename := range filesToRestore {
		cosRestoreFile(ctx, filename)
	}

	log.Println("completed")
}
