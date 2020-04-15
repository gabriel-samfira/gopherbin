package common

// Worker defines a simple worker interface
type Worker interface {
	Start() error
	Stop() error
}
