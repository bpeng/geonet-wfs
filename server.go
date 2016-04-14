package main

import (
	"database/sql"
	_ "github.com/GeoNet/cfg/cfgenv"
	_ "github.com/GeoNet/log/logentries"
	// "github.com/GeoNet/web"
	"github.com/NYTimes/gziphandler"
	_ "github.com/lib/pq"
	"log"
	"net/http"
	"os"
)

var (
	db *sql.DB
	// header web.Header
	maxOpenConns, maxIdleConns    int
	webServerProduction           bool
	webServerCname, webServerPort string
)

func init() {
	// libratoUser := os.Getenv("LIBRATO_USER")
	// libratoKey := os.Getenv("LIBRATO_KEY")
	// libratoSource := os.Getenv("LIBRATO_SOURCE")

	webServerProduction = os.Getenv("WEBSERVER_PRODUCTION") == "true"
	webServerCname = os.Getenv("WEBSERVER_CNAME")
	webServerPort = os.Getenv("WEBSERVER_PORT")
	maxOpenConns = 30
	maxIdleConns = 20

	// web.InitLibrato(libratoUser, libratoKey, libratoSource)
}

func dbOpenString() (dbstring string) {
	return os.ExpandEnv("host=${DB_HOST} " +
		"connect_timeout=${DB_CONN_TIMEOUT} " +
		"user=${DB_USER} " +
		"password=${DB_PASSWD} " +
		"dbname=${DB_NAME} " +
		"sslmode=${DB_SSLMODE}")
}

// main connects to the database, sets up request routing, and starts the http server.
func main() {
	var err error
	db, err = sql.Open("postgres", dbOpenString())
	if err != nil {
		log.Fatalf("Problem with DB config: %s\n", err)
	}
	defer db.Close()

	db.SetMaxIdleConns(maxIdleConns)
	db.SetMaxOpenConns(maxOpenConns)

	err = db.Ping()
	if err != nil {
		log.Println("Error: problem pinging DB - is it up and contactable?  500s will be served")
	}

	http.Handle("/", handler())
	log.Fatal(http.ListenAndServe(":"+webServerPort, nil))
}

func handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", toHandler(router))
	return gziphandler.GzipHandler(mux)
}
