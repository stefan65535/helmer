package logger

import (
	"fmt"
)

type Logger struct {
	buffer []string
	indent int
	Level  Level
}

type Level int

const (
	VERBOSE Level = 1
	ERROR   Level = 2
)

var Default Logger

func init() {
	Default = Logger{
		buffer: make([]string, 100),
		Level:  ERROR,
	}
}

func Verbose(indent int, msg string) {
	log(VERBOSE, indent, msg)
}

func Verbosef(indent int, format string, a ...any) {
	log(VERBOSE, indent, fmt.Sprintf(format, a...))
}

func Error(err error) {
	log(ERROR, 0, err.Error())
}

func log(level Level, indent int, msg string) {
	if Default.Level == VERBOSE {
		print(indent, msg)
	} else {
		switch level {
		case VERBOSE:
			// Store verbose info in case of error
			Default.buffer[indent] = msg
		case ERROR:
			for i := range Default.indent + 1 {
				print(i, Default.buffer[i])
			}
			print(indent, msg)
		}
	}

	Default.indent = indent
}

func print(indent int, msg string) {
	for range indent {
		fmt.Print("  ")
	}
	fmt.Println(msg)
}
