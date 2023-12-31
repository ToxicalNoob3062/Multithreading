package main

import (
	"database/sql"
	"encoding/gob"
	"gosub/data"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/alexedwards/scs/redisstore"
	"github.com/alexedwards/scs/v2"
	"github.com/gomodule/redigo/redis"
	_ "github.com/jackc/pgconn"
	_ "github.com/jackc/pgx/v4"
	_ "github.com/jackc/pgx/v4/stdlib"
)

const webPort = "8000"

func main() {
	//connect to database
	db := initDB()

	//create sessions
	session := initSession()

	//create loggers
	infoLog := log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
	errorLog := log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	//create channels
	errorChan := make(chan error)
	errorChanDone := make(chan bool)

	//create waitGroups
	wg := &sync.WaitGroup{}

	//setup App config
	app := Config{
		Session:       session,
		DB:            db,
		Wait:          wg,
		InfoLog:       infoLog,
		ErrorLog:      errorLog,
		Models:        data.New(db),
		ErrorChan:     errorChan,
		ErrorChanDone: errorChanDone,
	}

	//setup mail
	app.Mailer = app.createMail()
	go app.listenForMail()

	//listen for stop signals and shut down gracefully
	go app.listenForShutDown()

	//listen for errors
	go app.listenForErros()

	//listen for web connections
	app.spinServer()
}

func (app *Config) listenForErros() {
	for {
		select {
		case err := <-app.ErrorChan:
			app.ErrorLog.Println(err)
		case <-app.ErrorChanDone:
			return
		}
	}
}

func (app *Config) spinServer() {
	//start server
	srv := &http.Server{
		Addr:    ":" + webPort,
		Handler: app.routes(),
	}
	app.InfoLog.Printf("Starting server on port %s", webPort)
	err := srv.ListenAndServe()
	if err != nil {
		app.ErrorLog.Fatal(err)
	}
}

// For postgress DB!!
func initDB() *sql.DB {
	conn := connectToDB()
	if conn == nil {
		panic("failed to connect to database!!")
	}
	return conn
}

func connectToDB() *sql.DB {
	counts := 0

	dsn := os.Getenv("DSN")

	for {
		connection, err := openDB(dsn)
		if err != nil {
			println("Trying to reconnect to database!")
		} else {
			println("Connected to database!")
			return connection
		}

		if counts > 10 {
			return nil
		}

		println("Waiting for 1 second before trying again!")
		time.Sleep(time.Second * 1)
		counts++
	}
}

func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}

	if err = db.Ping(); err != nil {
		return nil, err
	}

	return db, nil
}

// For redis session
func initSession() *scs.SessionManager {
	//tell about the type of session we want to use
	gob.Register(data.User{})
	//setup session
	session := scs.New()
	session.Store = redisstore.New(initRedis())
	session.Lifetime = 24 * time.Hour
	session.Cookie.Persist = true
	session.Cookie.SameSite = http.SameSiteLaxMode
	session.Cookie.Secure = true

	return session
}

func initRedis() *redis.Pool {
	redisPool := &redis.Pool{
		MaxIdle: 10,
		Dial: func() (redis.Conn, error) {
			return redis.Dial("tcp", os.Getenv("REDIS"))
		},
	}
	return redisPool
}

func (app *Config) listenForShutDown() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	app.shutdown()
	os.Exit(0)
}

func (app *Config) shutdown() {
	app.InfoLog.Println("Running Clean Up Tasks!!...")

	//wait for all goroutines to finish (waitGroup)
	app.Wait.Wait()

	//Done with the application
	app.Mailer.DoneChan <- true
	app.ErrorChanDone <- true

	//close error log
	app.InfoLog.Println("Server gracefully shutdown! && closing channels!!...")

	//close channels
	close(app.Mailer.Mailerchan)
	close(app.Mailer.Errorchan)
	close(app.Mailer.DoneChan)
	close(app.ErrorChan)
	close(app.ErrorChanDone)

}

func (app *Config) createMail() Mail {
	mail := Mail{
		Domain:      "localhost",
		Host:        "localhost",
		Port:        1025,
		Encryption:  "none",
		FromAddress: "info@mycompany.com",
		FromName:    "Company",
		Errorchan:   make(chan error),
		Mailerchan:  make(chan Message, 100),
		Wait:        app.Wait,
		DoneChan:    make(chan bool),
	}

	return mail
}
