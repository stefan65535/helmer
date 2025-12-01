package logger

import (
	"testing"
)

func TestLogging(t *testing.T) {
	Log(VERBOSE, 0, "Hello0")
	Log(VERBOSE, 1, "Hello1")
	Log(VERBOSE, 2, "Hello2")
	Log(VERBOSE, 3, "Hello3")
	Log(ERROR, 0, "Error")
	Log(VERBOSE, 0, "Hello0 b1")
	Log(VERBOSE, 0, "Hello0 b2")
	Log(ERROR, 0, "Error")
	Log(VERBOSE, 0, "Hello0 c1")
	Log(VERBOSE, 3, "Hello1 c1")
	Log(ERROR, 0, "Error")
}
