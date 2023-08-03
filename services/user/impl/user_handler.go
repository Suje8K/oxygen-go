package impl

import (
	"context"
	"github.com/Suj8K/oxygen-go/services/sqlstore"
	"github.com/Suj8K/oxygen-go/services/user"
	"log"
	"math/rand"
	"net/http"
	"strconv"
)

func GetUser(w http.ResponseWriter, r *http.Request, store *sqlstore.SQLStore) *user.User {
	log.Println("Inside GetUser: ", store.GetEngine().DriverName())
	service, err := ProvideService(store)
	if err != nil {
		log.Println("Error in GetUser", err)
		return nil
	}

	user1, err := service.GetByID(context.Background(), &user.GetUserByIDQuery{ID: int64(10)})
	if err != nil {
		log.Println("Error 2", err)
		return nil
	}
	log.Println(user1.Email)
	return nil
}

func AddUserNew(w http.ResponseWriter, r *http.Request, store *sqlstore.SQLStore) *user.User {
	log.Println("Inside AddUserNew: ", store.GetEngine().DriverName())
	service, err := ProvideService(store)
	if err != nil {
		log.Println("Error in AddUserNew", err)
		return nil
	}

	uid := strconv.Itoa(rand.Intn(9999999))
	user1, err := service.Create(context.Background(), &user.CreateUserCommand{
		Email:   uid,
		Name:    uid,
		Login:   uid,
		Company: uid,
		IsAdmin: false,
	})
	if err != nil {
		log.Println("Error 2", err)
		return nil
	}
	log.Println(user1.Email)
	return nil
}
