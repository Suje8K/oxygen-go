package api

import (
	"encoding/json"
	"github.com/Suj8K/oxygen-go/services/sqlstore"
	"github.com/Suj8K/oxygen-go/services/user"
	"github.com/Suj8K/oxygen-go/services/user/impl"
	"github.com/gorilla/mux"
	"log"
	"net/http"
)

type apiFunc func(w http.ResponseWriter, r *http.Request) error

type apiFuncDB func(w http.ResponseWriter, r *http.Request, store *sqlstore.SQLStore) *user.User

type APIError struct {
	Error string
}

func WriteJSON(writer http.ResponseWriter, status int, v any) error {
	writer.WriteHeader(status)
	writer.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(writer).Encode(v)
}

func makeHttpHandlerFunc(f apiFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if err := f(writer, request); err != nil {
			WriteJSON(writer, http.StatusBadRequest, APIError{Error: err.Error()})
		}
	}
}

func dbHttpHandlerFunc(f apiFuncDB, store *sqlstore.SQLStore) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if user := f(writer, request, store); user != nil {
			WriteJSON(writer, http.StatusAccepted, user)
		}
	}
}

type APIServer struct {
	listenAddr string
	store      *sqlstore.SQLStore
}

func NewAPIServer(listenAddr string, store *sqlstore.SQLStore) *APIServer {
	return &APIServer{
		listenAddr: listenAddr,
		store:      store,
	}
}

func (s APIServer) Run() {
	router := mux.NewRouter()
	router.HandleFunc("/user/get", dbHttpHandlerFunc(impl.GetUser, s.store))
	router.HandleFunc("/user/add", dbHttpHandlerFunc(impl.AddUserNew, s.store))
	log.Println("JSON API running on port: ", s.listenAddr)
	log.Println("DB engine is: ", s.store.GetEngine().DriverName())
	err := http.ListenAndServe(s.listenAddr, router)
	if err != nil {
		log.Println("Error while running server: ", err)
	}
}
