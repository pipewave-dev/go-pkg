package entities

import (
	"time"
)

type User struct {
	ID string

	LastHeartbeat time.Time

	CreatedAt time.Time `dynamodbav:"create_at"`
}
