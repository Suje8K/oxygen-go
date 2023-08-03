package migrations

import (
	. "github.com/Suj8K/oxygen-go/services/sqlstore/migrator"
)

func addUserMigrations(mg *Migrator) {
	userV1 := Table{
		Name: "user",
		Columns: []*Column{
			{Name: "id", Type: DB_BigInt, IsPrimaryKey: true, IsAutoIncrement: true},
			{Name: "version", Type: DB_Int, Nullable: false},
			{Name: "login", Type: DB_NVarchar, Length: 190, Nullable: false},
			{Name: "email", Type: DB_NVarchar, Length: 190, Nullable: false},
			{Name: "name", Type: DB_NVarchar, Length: 255, Nullable: true},
			{Name: "password", Type: DB_NVarchar, Length: 255, Nullable: true},
			{Name: "salt", Type: DB_NVarchar, Length: 50, Nullable: true},
			{Name: "rands", Type: DB_NVarchar, Length: 50, Nullable: true},
			{Name: "company", Type: DB_NVarchar, Length: 255, Nullable: true},
			{Name: "account_id", Type: DB_BigInt, Nullable: false},
			{Name: "is_admin", Type: DB_Bool, Nullable: false},
			{Name: "created", Type: DB_DateTime, Nullable: false},
			{Name: "updated", Type: DB_DateTime, Nullable: false},
		},
		Indices: []*Index{
			{Cols: []string{"login"}, Type: UniqueIndex},
			{Cols: []string{"email"}, Type: UniqueIndex},
		},
	}

	// create table
	mg.AddMigration("create user table", NewAddTableMigration(userV1))
	// add indices
	mg.AddMigration("add unique index user.login", NewAddIndexMigration(userV1, userV1.Indices[0]))
	mg.AddMigration("add unique index user.email", NewAddIndexMigration(userV1, userV1.Indices[1]))
}
