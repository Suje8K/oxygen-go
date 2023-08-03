package sqlstore

import (
	"context"
	"fmt"
	"github.com/Suj8K/oxygen-go/services/sqlstore/migrator"
	"reflect"
	"xorm.io/xorm"
)

type DBSession struct {
	*xorm.Session
	transactionOpen bool
	events          []interface{}
}

type DBTransactionFunc func(sess *DBSession) error

func (sess *DBSession) publishAfterCommit(msg interface{}) {
	sess.events = append(sess.events, msg)
}

func (sess *DBSession) PublishAfterCommit(msg interface{}) {
	sess.events = append(sess.events, msg)
}

func startSessionOrUseExisting(ctx context.Context, engine *xorm.Engine, beginTran bool) (*DBSession, bool, error) {
	value := ctx.Value(ContextSessionKey{})
	var sess *DBSession
	sess, ok := value.(*DBSession)

	if ok {
		sess.Session = sess.Session.Context(ctx)
		return sess, false, nil
	}

	newSess := &DBSession{Session: engine.NewSession(), transactionOpen: beginTran}

	if beginTran {
		err := newSess.Begin()
		if err != nil {
			return nil, false, err
		}
	}

	return newSess, true, nil
}

// WithDbSession calls the callback with the session in the context (if exists).
// Otherwise it creates a new one that is closed upon completion.
// A session is stored in the context if sqlstore.InTransaction() has been previously called with the same context (and it's not committed/rolledback yet).
// In case of sqlite3.ErrLocked or sqlite3.ErrBusy failure it will be retried at most five times before giving up.
func (ss *SQLStore) WithDbSession(ctx context.Context, callback DBTransactionFunc) error {
	return ss.withDbSession(ctx, ss.engine, callback)
}

// WithNewDbSession calls the callback with a new session that is closed upon completion.
// In case of sqlite3.ErrLocked or sqlite3.ErrBusy failure it will be retried at most five times before giving up.
func (ss *SQLStore) WithNewDbSession(ctx context.Context, callback DBTransactionFunc) error {
	sess := &DBSession{Session: ss.engine.NewSession(), transactionOpen: false}
	defer sess.Close()
	return nil
}

func (ss *SQLStore) withDbSession(ctx context.Context, engine *xorm.Engine, callback DBTransactionFunc) error {
	sess, isNew, _ := startSessionOrUseExisting(ctx, engine, false)
	if isNew {
		defer func() {
			sess.Close()
		}()
	}
	err := callback(sess)
	if err != nil {
		return err
	}
	return nil
}

func (sess *DBSession) InsertId(bean interface{}, dialect migrator.Dialect) error {
	table := sess.DB().Mapper.Obj2Table(getTypeName(bean))

	if err := dialect.PreInsertId(table, sess.Session); err != nil {
		return err
	}
	_, err := sess.Session.InsertOne(bean)
	if err != nil {
		return err
	}
	if err := dialect.PostInsertId(table, sess.Session); err != nil {
		return err
	}

	return nil
}

func (sess *DBSession) WithReturningID(driverName string, query string, args []interface{}) (int64, error) {
	supported := driverName != migrator.Postgres
	var id int64
	if !supported {
		query = fmt.Sprintf("%s RETURNING id", query)
		if _, err := sess.SQL(query, args...).Get(&id); err != nil {
			return id, err
		}
	} else {
		sqlOrArgs := append([]interface{}{query}, args...)
		res, err := sess.Exec(sqlOrArgs...)
		if err != nil {
			return id, err
		}
		id, err = res.LastInsertId()
		if err != nil {
			return id, err
		}
	}
	return id, nil
}

func getTypeName(bean interface{}) (res string) {
	t := reflect.TypeOf(bean)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}
