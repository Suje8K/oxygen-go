package migrator

import (
	"fmt"
	"github.com/golang-migrate/migrate/v4/database"
	"log"
	"sync/atomic"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"xorm.io/xorm"
)

var (
	ErrMigratorIsLocked   = fmt.Errorf("migrator is locked")
	ErrMigratorIsUnlocked = fmt.Errorf("migrator is unlocked")
)

type Migrator struct {
	DBEngine     *xorm.Engine
	Dialect      Dialect
	migrations   []Migration
	migrationIds map[string]struct{}
	Logger       log.Logger
	isLocked     atomic.Bool
	logMap       map[string]MigrationLog
	tableName    string
}

type MigrationLog struct {
	Id          int64
	MigrationID string `xorm:"migration_id"`
	SQL         string `xorm:"sql"`
	Success     bool
	Error       string
	Timestamp   time.Time
}

func NewMigrator(engine *xorm.Engine) *Migrator {
	return NewScopedMigrator(engine, "")
}

// NewScopedMigrator should only be used for the transition to a new storage engine
func NewScopedMigrator(engine *xorm.Engine, scope string) *Migrator {
	mg := &Migrator{
		DBEngine:     engine,
		migrations:   make([]Migration, 0),
		migrationIds: make(map[string]struct{}),
		Dialect:      NewDialect(engine.DriverName()),
	}
	if scope == "" {
		mg.tableName = "migration_log"
		mg.Logger = log.Logger{}
	} else {
		mg.tableName = scope + "_migration_log"
		mg.Logger = log.Logger{}
	}
	return mg
}

// AddCreateMigration adds the initial migration log table -- this should likely be
// automatic and first, but enough tests exists that do not expect that we can keep it explicit
func (mg *Migrator) AddCreateMigration() {
	mg.AddMigration("create "+mg.tableName+" table", NewAddTableMigration(Table{
		Name: mg.tableName,
		Columns: []*Column{
			{Name: "id", Type: DB_BigInt, IsPrimaryKey: true, IsAutoIncrement: true},
			{Name: "migration_id", Type: DB_NVarchar, Length: 255},
			{Name: "sql", Type: DB_Text},
			{Name: "success", Type: DB_Bool},
			{Name: "error", Type: DB_Text},
			{Name: "timestamp", Type: DB_DateTime},
		},
	}))
}

func (mg *Migrator) MigrationsCount() int {
	return len(mg.migrations)
}

func (mg *Migrator) AddMigration(id string, m Migration) {
	if _, ok := mg.migrationIds[id]; ok {
		panic(fmt.Sprintf("migration id conflict: %s", id))
	}

	m.SetId(id)
	mg.migrations = append(mg.migrations, m)
	mg.migrationIds[id] = struct{}{}
}

func (mg *Migrator) GetMigrationIDs(excludeNotLogged bool) []string {
	result := make([]string, 0, len(mg.migrations))
	for _, migration := range mg.migrations {
		if migration.SkipMigrationLog() && excludeNotLogged {
			continue
		}
		result = append(result, migration.Id())
	}
	return result
}

func (mg *Migrator) GetMigrationLog() (map[string]MigrationLog, error) {
	logMap := make(map[string]MigrationLog)
	logItems := make([]MigrationLog, 0)

	exists, err := mg.DBEngine.IsTableExist(mg.tableName)
	if err != nil {
		return nil, fmt.Errorf("%v: %w", "failed to check table existence", err)
	}
	if !exists {
		return logMap, nil
	}

	if err = mg.DBEngine.Table(mg.tableName).Find(&logItems); err != nil {
		return nil, err
	}

	for _, logItem := range logItems {
		if !logItem.Success {
			continue
		}
		logMap[logItem.MigrationID] = logItem
	}

	mg.logMap = logMap
	return logMap, nil
}

func (mg *Migrator) RemoveMigrationLogs(migrationsIDs ...string) {
	for _, id := range migrationsIDs {
		delete(mg.logMap, id)
	}
}

func (mg *Migrator) Start(isDatabaseLockingEnabled bool, lockAttemptTimeout int) (err error) {
	if !isDatabaseLockingEnabled {
		return mg.run()
	}

	dbName, err := mg.Dialect.GetDBName(mg.DBEngine.DataSourceName())
	if err != nil {
		return err
	}
	key, err := database.GenerateAdvisoryLockId(dbName)
	if err != nil {
		return err
	}

	return mg.InTransaction(func(sess *xorm.Session) error {
		lockCfg := LockCfg{
			Session: sess,
			Key:     key,
			Timeout: lockAttemptTimeout,
		}

		if err := casRestoreOnErr(&mg.isLocked, false, true, ErrMigratorIsLocked, mg.Dialect.Lock, lockCfg); err != nil {
			return err
		}

		defer func() {
			unlockErr := casRestoreOnErr(&mg.isLocked, true, false, ErrMigratorIsUnlocked, mg.Dialect.Unlock, lockCfg)
			if unlockErr != nil {
			}
		}()

		// migration will run inside a nested transaction
		return mg.run()
	})
}

func (mg *Migrator) run() (err error) {

	_, err = mg.GetMigrationLog()
	if err != nil {
		return err
	}

	migrationsPerformed := 0
	migrationsSkipped := 0
	for _, m := range mg.migrations {
		m := m
		_, exists := mg.logMap[m.Id()]
		if exists {
			migrationsSkipped++
			continue
		}

		sql := m.SQL(mg.Dialect)

		record := MigrationLog{
			MigrationID: m.Id(),
			SQL:         sql,
			Timestamp:   time.Now(),
		}

		err := mg.InTransaction(func(sess *xorm.Session) error {
			err := mg.exec(m, sess)
			if err != nil {
				record.Error = err.Error()
				if !m.SkipMigrationLog() {
					if _, err := sess.Table(mg.tableName).Insert(&record); err != nil {
						return err
					}
				}
				return err
			}
			record.Success = true
			if !m.SkipMigrationLog() {
				_, err = sess.Table(mg.tableName).Insert(&record)
			}
			if err == nil {
				migrationsPerformed++
			}
			return err
		})
		if err != nil {
			return fmt.Errorf("%v: %w", fmt.Sprintf("migration failed (id = %s)", m.Id()), err)
		}
	}

	// Make sure migrations are synced
	return mg.DBEngine.Sync2()
}

func (mg *Migrator) exec(m Migration, sess *xorm.Session) error {

	condition := m.GetCondition()
	if condition != nil {
		sql, args := condition.SQL(mg.Dialect)

		if sql != "" {
			results, err := sess.SQL(sql, args...).Query()
			if err != nil {
				return err
			}

			if !condition.IsFulfilled(results) {
				return nil
			}
		}
	}

	var err error
	if codeMigration, ok := m.(CodeMigration); ok {
		err = codeMigration.Exec(sess, mg)
	} else {
		sql := m.SQL(mg.Dialect)
		_, err = sess.Exec(sql)
	}

	if err != nil {
		return err
	}

	return nil
}

type dbTransactionFunc func(sess *xorm.Session) error

func (mg *Migrator) InTransaction(callback dbTransactionFunc) error {
	sess := mg.DBEngine.NewSession()
	defer sess.Close()

	if err := sess.Begin(); err != nil {
		return err
	}

	if err := callback(sess); err != nil {
		if rollErr := sess.Rollback(); rollErr != nil {
			return fmt.Errorf("failed to roll back transaction due to error: %s: %w", rollErr, err)
		}

		return err
	}

	if err := sess.Commit(); err != nil {
		return err
	}

	return nil
}

func casRestoreOnErr(lock *atomic.Bool, o, n bool, casErr error, f func(LockCfg) error, lockCfg LockCfg) error {
	if !lock.CompareAndSwap(o, n) {
		return casErr
	}
	if err := f(lockCfg); err != nil {
		// Automatically unlock/lock on error
		lock.Store(o)
		return err
	}
	return nil
}
