package internal

import "math/rand"

func RandStr(n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"

	b := make([]byte, n)
	for i := range b {
		b[i] = chars[rand.Int()%len(chars)]
	}
	return string(b)
}
