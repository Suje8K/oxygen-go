package sqlstore

import (
	"errors"
	"fmt"
	"github.com/Suj8K/oxygen-go/services/sqlstore/migrator"
	"github.com/Suj8K/oxygen-go/services/sqlstore/session"
	"github.com/Suj8K/oxygen-go/services/sqlstore/sqlutil"
	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"log"
	"log/syslog"
	"os"
	"strings"
	"time"
	"xorm.io/xorm"
	"xorm.io/xorm/dialects"
	xormlog "xorm.io/xorm/log"
)

// ContextSessionKey is used as key to save values in `context.Context`
type ContextSessionKey struct{}

type DatabaseMigrator interface {
	AddMigration(mg *migrator.Migrator)
}

type SQLStore struct {
	dbCfg       DatabaseConfig
	log         xormlog.Logger
	engine      *xorm.Engine
	sqlxsession *session.SessionDB
	migrations  DatabaseMigrator
	Dialect     migrator.Dialect
}

func ProvideService(migrations DatabaseMigrator, isFeatureToggleEnabled bool) (*SQLStore, error) {
	// This change will make xorm use an empty default schema for postgres and
	// by that mimic the functionality of how it was functioning before
	// xorm's changes above.
	dialects.DefaultPostgresSchema = ""
	s, err := newSQLStore(nil, migrations)
	if err != nil {
		return nil, err
	}

	return s, nil
}

func newSQLStore(engine *xorm.Engine, migrations DatabaseMigrator) (*SQLStore, error) {
	ss := &SQLStore{
		migrations: migrations,
	}

	if err := ss.initEngine(engine); err != nil {
		return nil, fmt.Errorf("%v: %w", "se", err)
	}

	ss.Dialect = migrator.NewDialect(ss.engine.DriverName())

	return ss, nil
}

// Migrate performs database migrations.
// Has to be done in a second phase (after initialization), since other services can register migrations during
// the initialization phase.
func (ss *SQLStore) Migrate(isDatabaseLockingEnabled bool) error {
	if ss.dbCfg.SkipMigrations {
		return nil
	}

	migratorN := migrator.NewMigrator(ss.engine)
	ss.migrations.AddMigration(migratorN)

	return migratorN.Start(isDatabaseLockingEnabled, ss.dbCfg.MigrationLockAttemptTimeout)
}

// Sync syncs changes to the database.
func (ss *SQLStore) Sync() error {
	return ss.engine.Sync2()
}

// Reset resets database state.
// If default org and user creation is enabled, it will be ensured they exist in the database.
func (ss *SQLStore) Reset() error {
	return nil
}

// Quote quotes the value in the used SQL dialect
func (ss *SQLStore) Quote(value string) string {
	return ss.engine.Quote(value)
}

// GetDialect return the dialect
func (ss *SQLStore) GetDialect() migrator.Dialect {
	return ss.Dialect
}

func (ss *SQLStore) GetEngine() *xorm.Engine {
	return ss.engine
}

func (ss *SQLStore) GetSqlxSession() *session.SessionDB {
	if ss.sqlxsession == nil {
		ss.sqlxsession = session.GetSession(sqlx.NewDb(ss.engine.DB().DB, ss.GetDialect().DriverName()))
	}
	return ss.sqlxsession
}

func (ss *SQLStore) buildExtraConnectionString(sep rune) string {
	if ss.dbCfg.UrlQueryParams == nil {
		return ""
	}

	var sb strings.Builder
	for key, values := range ss.dbCfg.UrlQueryParams {
		for _, value := range values {
			sb.WriteRune(sep)
			sb.WriteString(key)
			sb.WriteRune('=')
			sb.WriteString(value)
		}
	}
	return sb.String()
}

func (ss *SQLStore) buildConnectionString() (string, error) {
	if err := ss.readConfig(); err != nil {
		return "", err
	}

	cnnstr := ss.dbCfg.ConnectionString

	// special case used by integration tests
	if cnnstr != "" {
		return cnnstr, nil
	}

	switch ss.dbCfg.Type {
	case migrator.Postgres:
		addr, err := sqlutil.SplitHostPortDefault(ss.dbCfg.Host, "127.0.0.1", "5432")
		if err != nil {
			return "", fmt.Errorf("invalid host specifier '%s': %w", ss.dbCfg.Host, err)
		}

		args := []any{ss.dbCfg.User, addr.Host, addr.Port, ss.dbCfg.Name, ss.dbCfg.SslMode, ss.dbCfg.ClientCertPath,
			ss.dbCfg.ClientKeyPath, ss.dbCfg.CaCertPath}
		for i, arg := range args {
			if arg == "" {
				args[i] = "''"
			}
		}
		cnnstr = fmt.Sprintf("user=%s host=%s port=%s dbname=%s sslmode=%s sslcert=%s sslkey=%s sslrootcert=%s", args...)
		if ss.dbCfg.Pwd != "" {
			cnnstr += fmt.Sprintf(" password=%s", ss.dbCfg.Pwd)
		}

		cnnstr += ss.buildExtraConnectionString(' ')
	default:
		return "", fmt.Errorf("n database type: %s", ss.dbCfg.Type)
	}

	return cnnstr, nil
}

// initEngine initializes ss.engine.
func (ss *SQLStore) initEngine(engine *xorm.Engine) error {
	if ss.engine != nil {
		return nil
	}

	// connStr := "postgres://oxygenuser:oxygenpass@localhost:5432/oxygendb?sslmode=verify-full
	connectionString, err := ss.buildConnectionString()
	if err != nil {
		return err
	}

	if engine == nil {
		var err error
		engine, err = xorm.NewEngine(ss.dbCfg.Type, connectionString)
		if err != nil {
			return err
		}
	}

	engine.SetMaxOpenConns(ss.dbCfg.MaxOpenConn)
	engine.SetMaxIdleConns(ss.dbCfg.MaxIdleConn)
	engine.SetConnMaxLifetime(time.Second * time.Duration(ss.dbCfg.ConnMaxLifetime))

	// configure sql logging
	var debugSQL = false
	if !debugSQL {
		engine.SetLogger(&xormlog.DiscardLogger{})
	} else {
		logWriter, err := syslog.New(syslog.LOG_DEBUG, "sqlstore")
		if err != nil {
			log.Fatalf("Fail to create xorm system logger: %v\n", err)
		}
		// add stack to database calls to be able to see what repository initiated queries. Top 7 items from the stack as they are likely in the xorm library.
		engine.SetLogger(xormlog.NewSimpleLogger(logWriter))
		engine.ShowSQL(true)
	}

	ss.engine = engine
	return nil
}

// The transaction_isolation system variable isn't compatible with MySQL < 5.7.20 or MariaDB. If we get an error saying this
// system variable is unknown, then replace it with it's older version tx_isolation which is compatible with MySQL < 5.7.20 and MariaDB.
func (ss *SQLStore) ensureTransactionIsolationCompatibility(engine *xorm.Engine, connectionString string) (*xorm.Engine, error) {
	var result string
	_, err := engine.SQL("SELECT 1").Get(&result)

	var mysqlError *mysql.MySQLError
	if errors.As(err, &mysqlError) {
		// if there was an error due to transaction isolation
		if strings.Contains(mysqlError.Message, "Unknown system variable 'transaction_isolation'") {
			ss.log.Debug("transaction_isolation system var is unknown, overriding in connection string with tx_isolation instead")
			// replace with compatible system var for transaction isolation
			connectionString = strings.Replace(connectionString, "&transaction_isolation", "&tx_isolation", -1)
			// recreate the xorm engine with new connection string that is compatible
			engine, err = xorm.NewEngine(ss.dbCfg.Type, connectionString)
			if err != nil {
				return nil, err
			}
		}
	} else if err != nil {
		return nil, err
	}

	return engine, nil
}

// readConfig initializes the SQLStore from its configuration.
func (ss *SQLStore) readConfig() error {
	//postgres: //oxygenuser:oxygenpass@localhost:5432/oxygendb?sslmode=verify-full

	ss.dbCfg.Type = "http"
	ss.dbCfg.Host = "localhost:5432"
	ss.dbCfg.Name = "oxygendb"
	ss.dbCfg.User = "oxygenuser"
	ss.dbCfg.Pwd = "oxygenpass"
	ss.dbCfg.SslMode = "disable"
	ss.dbCfg.Type = migrator.Postgres

	//
	//ss.dbCfg.MaxOpenConn = sec.Key("max_open_conn").MustInt(0)
	//ss.dbCfg.MaxIdleConn = sec.Key("max_idle_conn").MustInt(2)
	//ss.dbCfg.ConnMaxLifetime = sec.Key("conn_max_lifetime").MustInt(14400)
	//
	//ss.dbCfg.SslMode = sec.Key("ssl_mode").String()
	//ss.dbCfg.CaCertPath = sec.Key("ca_cert_path").String()
	//ss.dbCfg.ClientKeyPath = sec.Key("client_key_path").String()
	//ss.dbCfg.ClientCertPath = sec.Key("client_cert_path").String()
	//ss.dbCfg.ServerCertName = sec.Key("server_cert_name").String()
	//ss.dbCfg.Path = sec.Key("path").MustString("data/grafana.db")
	//ss.dbCfg.IsolationLevel = sec.Key("isolation_level").String()
	//
	//ss.dbCfg.CacheMode = sec.Key("cache_mode").MustString("private")
	//ss.dbCfg.WALEnabled = sec.Key("wal").MustBool(false)
	//ss.dbCfg.SkipMigrations = sec.Key("skip_migrations").MustBool()
	//ss.dbCfg.MigrationLockAttemptTimeout = sec.Key("locking_attempt_timeout_sec").MustInt()
	//
	//ss.dbCfg.QueryRetries = sec.Key("query_retries").MustInt()
	//ss.dbCfg.TransactionRetries = sec.Key("transaction_retries").MustInt(5)
	return nil
}

func (ss *SQLStore) GetMigrationLockAttemptTimeout() int {
	return ss.dbCfg.MigrationLockAttemptTimeout
}

func IsTestDbPostgres() bool {
	if db, present := os.LookupEnv("GRAFANA_TEST_DB"); present {
		return db == migrator.Postgres
	}

	return false
}

type DatabaseConfig struct {
	Type                        string
	Host                        string
	Name                        string
	User                        string
	Pwd                         string
	Path                        string
	SslMode                     string
	CaCertPath                  string
	ClientKeyPath               string
	ClientCertPath              string
	ServerCertName              string
	ConnectionString            string
	IsolationLevel              string
	MaxOpenConn                 int
	MaxIdleConn                 int
	ConnMaxLifetime             int
	CacheMode                   string
	WALEnabled                  bool
	UrlQueryParams              map[string][]string
	SkipMigrations              bool
	MigrationLockAttemptTimeout int
	// SQLite only
	QueryRetries int
	// SQLite only
	TransactionRetries int
}
