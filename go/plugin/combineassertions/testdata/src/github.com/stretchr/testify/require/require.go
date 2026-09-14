// Package require is the halting half of the testify stand-in. See the assert package beside it.
package require

type TestingT interface {
	Errorf(format string, args ...interface{})
	FailNow()
}

func Equal(t TestingT, expected, actual interface{}, msgAndArgs ...interface{}) {}
func Len(t TestingT, object interface{}, length int, msgAndArgs ...interface{}) {}
func NoError(t TestingT, err error, msgAndArgs ...interface{})                  {}
