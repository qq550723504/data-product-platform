package application

import (
	"fmt"

	"github.com/google/uuid"
)

func parseExecutionID(value string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, fmt.Errorf("parse execution id %q: %w", value, err)
	}
	return parsed, nil
}
