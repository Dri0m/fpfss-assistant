package main

import (
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
	"io"
	"time"
)

func (a *app) initLogger() {
	mw := io.MultiWriter( /*os.Stdout,*/ &lumberjack.Logger{
		Filename:   "log.log",
		MaxSize:    100, // megabytes
		MaxAge:     0,   //days
		MaxBackups: 0,
		Compress:   true,
	})
	l := logrus.New()
	l.SetFormatter(&logrus.TextFormatter{
		DisableColors:   true,
		FullTimestamp:   true,
		TimestampFormat: time.RFC3339Nano,
	})
	l.SetOutput(mw)
	l.SetLevel(logrus.TraceLevel)
	l.SetReportCaller(true)
	a.l = l
}

func (a *app) fatalErr(err error) {
	if err != nil {
		fmt.Printf("error: %v\n", err)
		a.l.Fatal(err)
	}
}

func (a *app) openDB() {
	a.l.Infof("opening database '%s'...", dbName)
	db, err := sql.Open("sqlite3", dbName)
	a.fatalErr(err)
	err = db.Ping()
	a.fatalErr(err)

	db.SetMaxOpenConns(1)

	_, err = db.Exec(`PRAGMA journal_mode = WAL`)
	a.fatalErr(err)

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS data (
			id INT PRIMARY KEY,
			status TEXT,
			file_url TEXT,
			sha256 TEXT
		)
		`)
	a.fatalErr(err)

	a.db = db
}
