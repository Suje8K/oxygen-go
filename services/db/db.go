package db

import (
	"context"
	"github.com/Suj8K/oxygen-go/services/sqlstore"
	"github.com/Suj8K/oxygen-go/services/sqlstore/migrator"
	"github.com/Suj8K/oxygen-go/services/sqlstore/session"
	"os"
)

type DB interface {
	//WithTransactionalDbSession(ctx context.Context, callback sqlstore.DBTransactionFunc) error
	WithDbSession(ctx context.Context, callback sqlstore.DBTransactionFunc) error
	WithNewDbSession(ctx context.Context, callback sqlstore.DBTransactionFunc) error
	GetDialect() migrator.Dialect
	//GetDBType() core.DbType
	GetSqlxSession() *session.SessionDB
	//InTransaction(ctx context.Context, fn func(ctx context.Context) error) error
	Quote(value string) string
	// RecursiveQueriesAreSupported runs a dummy recursive query and it returns true
	// if the query runs successfully or false if it fails with mysqlerr.ER_PARSE_ERROR error or any other error
	//RecursiveQueriesAreSupported() (bool, error)
}

type Session = sqlstore.DBSession

var ProvideService = sqlstore.ProvideService

func IsTestDbPostgres() bool {
	if db, present := os.LookupEnv("GRAFANA_TEST_DB"); present {
		return db == migrator.Postgres
	}

	return false
}
