package migrations

import . "github.com/Suj8K/oxygen-go/services/sqlstore/migrator"

type OxygenMigrations struct {
}

func ProvideOSSMigrations() *OxygenMigrations {
	return &OxygenMigrations{}
}

func (*OxygenMigrations) AddMigration(mg *Migrator) {
	mg.AddCreateMigration()
	addUserMigrations(mg)
}
