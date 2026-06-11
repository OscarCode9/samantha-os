package cron

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"time"
)

// generateJobID genera un ID único para un job.
func generateJobID() string {
	return fmt.Sprintf("cron_%d_%s", time.Now().UnixNano(), randomString(8))
}

// generateRunLogID genera un ID único para un log de ejecución.
func generateRunLogID() string {
	return fmt.Sprintf("log_%d_%s", time.Now().UnixNano(), randomString(8))
}

// randomString genera una cadena aleatoria de N caracteres.
func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		val, _ := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		b[i] = letters[val.Int64()]
	}
	return string(b)
}
