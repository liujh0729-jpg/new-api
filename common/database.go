package common

const (
	DatabaseTypeMySQL      = "mysql"
	DatabaseTypeSQLite     = "sqlite"
	DatabaseTypePostgreSQL = "postgres"
)

var UsingSQLite = false
var UsingPostgreSQL = false
var LogSqlType = DatabaseTypeSQLite // Default to SQLite for logging SQL queries
var UsingMySQL = false
var UsingClickHouse = false

// SQLitePath is the default SQLite DSN. This project opens SQLite through the
// pure-Go driver, which only applies pragmas given as repeated _pragma query
// parameters, so a mattn-style "_busy_timeout=30000" would be silently ignored.
// busy_timeout makes concurrent writers wait instead of failing with "database
// is locked"; WAL lets a reader never block a writer.
var SQLitePath = "one-api.db?_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)"
