package uuidv7

import (
	"github.com/google/uuid"
)

// New generates a new UUIDv7 (time-ordered 128-bit identifier).
func New() (uuid.UUID, error) {
	return uuid.NewV7()
}

// MustNew generates a new UUIDv7, panicking on failure (e.g. system entropy error).
func MustNew() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		panic("failed to generate uuidv7: " + err.Error())
	}
	return id
}

// Parse parses a string into a uuid.UUID.
func Parse(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

// IsValid checks if the string is a valid UUID format.
func IsValid(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}
