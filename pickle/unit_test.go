package pickle

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestByteArray(t *testing.T) {
	in, _ := hex.DecodeString("8004950e00000000000000430a74657374206279746573942e") // pickle.dumps(b'test bytes').hex()
	u := NewUnpickler(in)
	obj, err := u.Load()
	if err != nil {
		t.Error(err)
		return
	}

	var b strings.Builder
	obj.JSON(&b)
	t.Logf("(%T) %v -> JSON %s", obj, obj, b.String())

	if b.String() != `"dGVzdCBieXRlcw=="` {
		t.Errorf("unexpected JSON value (%T) % x (%q) -> JSON %s", obj, obj, obj, b.String())
	}
}
