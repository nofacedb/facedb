package storages

import (
	"database/sql"
	"fmt"

	"github.com/kshvakov/clickhouse"
	"github.com/nofacedb/facedb/internal/cfgparser"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// CreateClickHouseDBConn ...
func CreateClickHouseDBConn(cfg *cfgparser.StorageCFG, logger *log.Logger) (*sql.DB, error) {
	connStr := createConnStr(cfg)
	db, err := connectToClickHouseDB(connStr, cfg.MaxPings, logger)
	if err != nil {
		return nil, errors.Wrap(err, "unable to connect to ClickHouse DB")
	}
	return db, nil
}

func createConnStr(cfg *cfgparser.StorageCFG) string {
	connStr := fmt.Sprintf("tcp://%s:%d?username=%s&password=%s&database=%s&read_timeout=%d&write_timeout=%d&debug=%v",
		cfg.Addr, cfg.Port, cfg.User, cfg.Password, cfg.DefaultDB, cfg.ReadTimeoutMS/1000, cfg.WriteTimeoutMS/1000, cfg.Debug)
	return connStr
}

func connectToClickHouseDB(connStr string, maxPings int, logger *log.Logger) (*sql.DB, error) {
	db, err := sql.Open("clickhouse", connStr)
	if err != nil {
		return nil, errors.Wrap(err, "unable to open connection to ClickHouse DB")
	}
	pingTimes := 0
	for pingTimes = 0; pingTimes < maxPings; pingTimes++ {
		err := db.Ping()
		if err == nil {
			break
		}
		if exception, ok := err.(*clickhouse.Exception); ok {
			logger.Errorf("ClickHouse DB exception: [%d] %s \n%s\n", exception.Code, exception.Message, exception.StackTrace)
		} else {
			logger.Error(errors.Wrapf(err, "unable to ping ClickHouse DB for %d time", pingTimes+1))
		}
	}
	if pingTimes == maxPings {
		return nil, fmt.Errorf("unable to ping ClickHouse DB for %d times", maxPings)
	}

	return db, nil
}
