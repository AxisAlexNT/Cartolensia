package media

import (
	"strings"
	"testing"
)

func TestHashReaderStreamsSHA512(t *testing.T) {
	got, err := HashReader(strings.NewReader("cartolensia"))
	if err != nil {
		t.Fatal(err)
	}
	const want = "9e20f086fe4d60f804b9a36d90833565cb383be4fe084fd19a1565aa8de0dcbddead3ed4e97d5a1d64fc949d35bf592bc40ff36c34a2468b7c6d1e8ef40f7527"
	if got.SHA512Hex != want {
		t.Fatalf("unexpected hash %s", got.SHA512Hex)
	}
	if got.Bytes != int64(len("cartolensia")) {
		t.Fatalf("unexpected byte count %d", got.Bytes)
	}
}
