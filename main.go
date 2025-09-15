package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"os"
	"time"

	"github.com/grafana/loki/v3/pkg/logql/syntax"
	"github.com/grafana/loki/v3/pkg/loki"
	"github.com/grafana/loki/v3/pkg/querier/plan"
	"github.com/grafana/loki/v3/pkg/storage/chunk"
	"github.com/grafana/loki/v3/pkg/util/cfg"
	"github.com/prometheus/common/model"
	"github.com/yankeguo/rg"
)

func main() {
	var err error
	defer func() {
		if err == nil {
			return
		}
		log.Println("exited with error:", err.Error())
		os.Exit(1)
	}()
	defer rg.Guard(&err)

	var (
		optConf    string
		optUserID  string
		optTimeBeg string
		optTimeEnd string
		optQuery   string
	)
	flag.StringVar(&optConf, "conf", "loki.yml", "Loki configuration file")
	flag.StringVar(&optUserID, "user-id", "fake", "Loki tenant user id")
	flag.StringVar(&optTimeBeg, "time-beg", "", "time range begin time, RFC3339 format")
	flag.StringVar(&optTimeEnd, "time-end", "", "time range end time, RFC3339 format")
	flag.StringVar(&optQuery, "query", "", "Loki query")
	flag.Parse()

	if optTimeBeg == "" {
		err = errors.New("missing -time-beg")
		return
	}

	if optTimeEnd == "" {
		err = errors.New("missing -time-end")
		return
	}

	timeBeg := rg.Must(time.Parse(time.RFC3339, optTimeBeg))

	timeEnd := rg.Must(time.Parse(time.RFC3339, optTimeEnd))

	if optQuery == "" {
		err = errors.New("missing -query")
		return
	}

	var config loki.ConfigWrapper
	rg.Must0(cfg.YAML(optConf, true, false)(&config))
	rg.Must0(config.Validate())

	ins := rg.Must(loki.New(config.Config))

	expr := rg.Must(syntax.ParseExpr(optQuery))

	matchers := rg.Must(syntax.ParseMatchers(optQuery, true))

	predicate := chunk.NewPredicate(matchers, &plan.QueryPlan{
		AST: expr,
	})

	chunksGroup, _ := rg.Must2(ins.Store.GetChunks(
		context.Background(),
		optUserID,
		model.TimeFromUnix(timeBeg.Unix()),
		model.TimeFromUnix(timeEnd.Unix()),
		predicate,
		nil,
	))

	for i, chunks := range chunksGroup {
		for j, chunk := range chunks {
			log.Printf(">>>>>>>>>> %d/%d\n%s", i, j, rg.Must(json.MarshalIndent(chunk, "", "  ")))
		}
	}
}
