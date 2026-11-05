package pickle

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/mistsys/gopickle2json/types"
)

func TestRawUnicodeEscape(t *testing.T) {
	// python string `"café'\t🐍…\\\n` (has upper latin-1, control and unicode chars) pickled in version 0 encoding to force raw-unicode-escape encoding
	in, _ := hex.DecodeString("5622636166e927095c5530303031663430645c75323032365c75303035635c75303030610a70300a2e") // pickle.dumps('"café\'\t🐍…\\\n', 0).hex()
	u := NewUnpickler(in)
	obj, err := u.Load()
	if err != nil {
		t.Error(err)
		return
	}
	if s, ok := obj.(*types.EscapedString); !ok {
		t.Errorf("unexpected output type (%T) %v", obj, obj)
	} else if s.String() != "\"café'\t🐍…\\\n" {
		t.Errorf("unexpected output value (%T) % x (%q) (%s)", obj, string(*s), s, string(*s))
	} else {
		var b strings.Builder
		obj.JSON(&b)
		t.Logf("(%T) % x (%q) -> JSON %s", obj, obj, obj, b.String())
		if b.String() != `"\"café'\t🐍…\\\n"` {
			t.Errorf("unexpected JSON value (%T) % x (%q) -> JSON %s", obj, obj, obj, b.String())
		}
	}
}

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
