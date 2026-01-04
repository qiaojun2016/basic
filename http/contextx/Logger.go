package contextx

import "log"

type Logger struct {
	debug bool
}

func (l *Logger) Debugf(format string, v ...interface{}) {
	if l.debug {
		log.Printf("DEBUG: "+format, v...)
	}
}
