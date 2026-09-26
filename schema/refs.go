package schema

import "aego/core/ids"

type AssetRef interface {
	AssetRefID() ids.AssetID
	SetAssetRefID(ids.AssetID)
	AssetKind() string
}

type NodeRef interface {
	NodeRefID() ids.NodeID
	SetNodeRefID(ids.NodeID)
}
