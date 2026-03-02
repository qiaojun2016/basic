package mysql

import (
	"database/sql"
	"log"
	"time"
)

// timingDBExec 包装 DBExec，记录每次查询的耗时
type timingDBExec struct {
	inner     DBExec
	threshold int64 // ms，超过此值才打印
}

func (t *timingDBExec) logTiming(op string, start time.Time) {
	elapsed := time.Since(start).Milliseconds()
	if elapsed >= t.threshold {
		log.Printf("[MySQL] %s %dms", op, elapsed)
	}
}

func (t *timingDBExec) Exec(query string, args ...interface{}) (sql.Result, error) {
	defer t.logTiming(query, time.Now())
	return t.inner.Exec(query, args...)
}

func (t *timingDBExec) NamedExec(query string, arg interface{}) (sql.Result, error) {
	defer t.logTiming(query, time.Now())
	return t.inner.NamedExec(query, arg)
}

func (t *timingDBExec) Get(dest interface{}, query string, args ...interface{}) error {
	defer t.logTiming(query, time.Now())
	return t.inner.Get(dest, query, args...)
}

func (t *timingDBExec) Select(dest interface{}, query string, args ...interface{}) error {
	defer t.logTiming(query, time.Now())
	return t.inner.Select(dest, query, args...)
}

func (t *timingDBExec) Rebind(query string) string {
	return t.inner.Rebind(query)
}
