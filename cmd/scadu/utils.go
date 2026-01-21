package main

import (
	"fmt"

	"github.com/google/uuid"
)

// GenerateCommitMessage appends a unique SCADUFAX_ID to the commit message.
func GenerateCommitMessage(msg string) string {
	id := uuid.New().String()
	return fmt.Sprintf("%s\n\nSCADUFAX_ID: %s", msg, id)
}
