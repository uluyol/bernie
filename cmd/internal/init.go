package internal

import (
	"math/rand"
	"time"
)

// Properly seed so we have different bernie names
func init() {
	rand.Seed(time.Now().UnixNano())
}
