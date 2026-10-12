package ids

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"

	"aego/core/errs"
	"aego/core/log"
)

const HexLen = 32

type NodeID struct{ Hi, Lo uint64 }

type AssetID struct{ Hi, Lo uint64 }

func newRaw() (uint64, uint64) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("ids: crypto/rand failed: " + err.Error())
	}
	return binary.BigEndian.Uint64(b[0:8]), binary.BigEndian.Uint64(b[8:16])
}

func NewNodeID() NodeID {
	hi, lo := newRaw()
	return NodeID{Hi: hi, Lo: lo}
}

func NewAssetID() AssetID {
	hi, lo := newRaw()
	return AssetID{Hi: hi, Lo: lo}
}

func format(hi, lo uint64) string {
	var b [16]byte
	binary.BigEndian.PutUint64(b[0:8], hi)
	binary.BigEndian.PutUint64(b[8:16], lo)
	return hex.EncodeToString(b[:])
}

func parse(s string) (uint64, uint64, error) {
	if len(s) != HexLen {
		return 0, 0, errs.New(errs.IDBadFormat, "identifier must be 32 hex characters",
			log.F("len", len(s)))
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return 0, 0, errs.Wrap(err, errs.IDBadFormat, "identifier is not valid hex")
	}
	return binary.BigEndian.Uint64(b[0:8]), binary.BigEndian.Uint64(b[8:16]), nil
}

func (id NodeID) String() string {
	return format(id.Hi, id.Lo)
}

func (id NodeID) IsZero() bool {
	return id.Hi == 0 && id.Lo == 0
}

func (id NodeID) Short() string {
	if id.IsZero() {
		return "nil"
	}
	return id.String()[:8]
}

func (id AssetID) String() string {
	return format(id.Hi, id.Lo)
}

func (id AssetID) IsZero() bool {
	return id.Hi == 0 && id.Lo == 0
}

func (id AssetID) Short() string {
	if id.IsZero() {
		return "nil"
	}
	return id.String()[:8]
}

func ParseNodeID(s string) (NodeID, error) {
	hi, lo, err := parse(s)
	if err != nil {
		return NodeID{}, err
	}
	return NodeID{Hi: hi, Lo: lo}, nil
}

func ParseAssetID(s string) (AssetID, error) {
	hi, lo, err := parse(s)
	if err != nil {
		return AssetID{}, err
	}
	return AssetID{Hi: hi, Lo: lo}, nil
}
