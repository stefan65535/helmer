package logger

import (
	"testing"
)

func TestLogging(t *testing.T) {
	log(VERBOSE, 0, "Hello0")
	log(VERBOSE, 1, "Hello1")
	log(VERBOSE, 2, "Hello2")
	log(VERBOSE, 3, "Hello3")
	log(ERROR, 0, "Error")
	log(VERBOSE, 0, "Hello0 b1")
	log(VERBOSE, 0, "Hello0 b2")
	log(ERROR, 0, "Error")
	log(VERBOSE, 0, "Hello0 c1")
	log(VERBOSE, 3, "Hello1 c1")
	log(ERROR, 0, "Error")
}
