// Package assert is a stand-in for testify's, holding only the signatures the fixtures call.
//
// analysistest resolves imports out of testdata/src rather than the module cache, so the
// analyzer sees this package under testify's real import path, which is what it matches on.
// Only the shapes matter: which argument is the value under test, and whether the receiver
// carries the TestingT.
package assert

type TestingT interface {
	Errorf(format string, args ...interface{})
}

func Equal(t TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool { return true }
func Len(t TestingT, object interface{}, length int, msgAndArgs ...interface{}) bool { return true }
func True(t TestingT, value bool, msgAndArgs ...interface{}) bool                    { return true }
func Empty(t TestingT, object interface{}, msgAndArgs ...interface{}) bool           { return true }
func NoError(t TestingT, err error, msgAndArgs ...interface{}) bool                  { return true }

type Assertions struct{ t TestingT }

func New(t TestingT) *Assertions { return &Assertions{t: t} }

func (a *Assertions) Equal(expected, actual interface{}, msgAndArgs ...interface{}) bool {
	return true
}
func (a *Assertions) Len(object interface{}, length int, msgAndArgs ...interface{}) bool { return true }
