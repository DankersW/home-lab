package bucket_test

import (
	"testing"

	"github.com/dankers/home-lab/services/receipts/internal/db/bucket"
	"github.com/stretchr/testify/require"
)

func TestKey(t *testing.T) {
	require.Equal(t, "r1/a1", bucket.Key("r1", "a1"))
	// Both IDs are UUIDs in practice, so the key is collision-free and the
	// receipt prefix lets a whole receipt be swept by prefix.
	require.Equal(t, "rid-uuid/att-uuid", bucket.Key("rid-uuid", "att-uuid"))
}
