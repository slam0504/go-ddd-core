package storagetest

import (
	"testing"

	"github.com/slam0504/go-ddd-core/ports/storage"
)

// TestReferenceImplementationPassesContract proves RunContract is
// satisfiable: the suite is only trustworthy if a known-good implementation
// passes it.
func TestReferenceImplementationPassesContract(t *testing.T) {
	RunContract(t, func(t *testing.T) storage.ObjectStorage {
		return newMemStorage()
	})
}
