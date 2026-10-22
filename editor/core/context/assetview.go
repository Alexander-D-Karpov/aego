package context

import "aego/core/ids"

type AssetView func(*Editor, ids.AssetID)

// AssetPlayer auditions assets of one artifact kind or data type, such as sounds in an asset picker.
type AssetPlayer struct {
	Play    func(*Editor, ids.AssetID) error
	Stop    func(*Editor)
	Playing func(*Editor) ids.AssetID
}
