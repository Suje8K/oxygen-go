package main

import (
	"fmt"
	"github.com/Suj8K/oxygen-go/api"
	"github.com/Suj8K/oxygen-go/services/sqlstore"
	"github.com/Suj8K/oxygen-go/services/sqlstore/migrations"
)

func main() {

	// Init DB service
	dbService, dbError := sqlstore.ProvideService(&migrations.OxygenMigrations{}, false)
	if dbError != nil {
		fmt.Println(dbError)
	}

	// Perform Migrations
	err := dbService.Migrate(false)
	if err != nil {
		fmt.Println(err)
	}

	// Run Http server
	apiServer := api.NewAPIServer(":9096", dbService)
	apiServer.Run()
}
