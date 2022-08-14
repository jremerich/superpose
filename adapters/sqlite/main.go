package sqlite

import (
	"database/sql"
	"log"
	"superpose-sync/adapters/ConfigFile"
	"superpose-sync/utils"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var (
	file string
	DB   DBConnection
)

type Stmt struct {
	stmt *sql.Stmt
}

func (s Stmt) Exec(args ...any) (sql.Result, error) {
	return s.stmt.Exec(args...)
}

func (s Stmt) Close() error {
	//return s.stmt.Close()
	return nil
}

type DBConnection struct {
	conn  *sql.DB
	debug bool
}

type DebugEntry struct {
	query     string
	method    string
	duration  time.Duration
	startedAt time.Time
	stoppedAt time.Time
}

func (debugEntry DebugEntry) startTimer() {
	debugEntry.startedAt = time.Now()
}

func (debugEntry DebugEntry) finish() {
	debugEntry.stopTimer()
	if DB.debug {
		log.Println("sql debug: ", debugEntry)
	}
}

func (debugEntry DebugEntry) stopTimer() {
	debugEntry.stoppedAt = time.Now()
	debugEntry.duration = debugEntry.stoppedAt.Sub(debugEntry.startedAt)
}

func (c DBConnection) Exec(sql string) (sql.Result, error) {
	result, err := c.conn.Exec(sql)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (c DBConnection) Query(sql string) (*sql.Rows, error) {
	result, err := c.conn.Query(sql)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (c DBConnection) QueryRow(query string, args ...any) *sql.Row {
	debug := DebugEntry{
		query:  query,
		method: utils.GetFunctionName(),
	}
	debug.startTimer()

	result := c.conn.QueryRow(query, args...)

	debug.finish()

	return result
}

func (c DBConnection) Prepare(query string) (*Stmt, error) {
	stmt, err := c.conn.Prepare(query)
	return &Stmt{stmt: stmt}, err
}

func Connect() {
	file = ConfigFile.Configs.DbPath
	log.Println("Initiating SQLite3 for file ", file)

	db, err := sql.Open("sqlite3", file)
	if err != nil {
		log.Fatalf("Unable to connect to sqlite file: %v", err)
	}

	DB = DBConnection{conn: db, debug: false}
}
