package tools

import (
	"time"

	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

// pushRequest builds a models.Request for pushing transactions.
func pushRequest(serverTimestamp int, transactions []models.Transaction) models.Request {
	return models.Request{
		CurrentClientTimestamp: int(time.Now().Unix()),
		ServerTimestamp:        serverTimestamp,
		Transaction:            transactions,
	}
}

// pushTagRequest builds a models.Request for pushing categories.
func pushTagRequest(serverTimestamp int, tags []models.Tag) models.Request {
	return models.Request{
		CurrentClientTimestamp: int(time.Now().Unix()),
		ServerTimestamp:        serverTimestamp,
		Tag:                    tags,
	}
}
