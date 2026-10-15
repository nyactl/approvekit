//go:build !darwin && !linux

package people

// lock does nothing where there is no file locking; programs that share a
// label file at the same time may then give one label to two people.
func lock(string) (func(), error) { return func() {}, nil }
