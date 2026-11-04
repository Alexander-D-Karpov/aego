package pipeline

import "aego/asset"

// Source metadata is shared by importer, Inspector and semantic mutations.
// Runtime loaders continue to consume prepared artifacts only.
const MetaVersion = asset.MetaVersion
const MetaSuffix = asset.MetaSuffix

type Settings = asset.ImportSettings
type SubAsset = asset.SourceFrame
type Meta = asset.SourceMeta

var NewMeta = asset.NewSourceMeta
var ParseMeta = asset.ParseSourceMeta
var WriteMeta = asset.MarshalSourceMeta
var canonicalSubs = asset.MarshalSourceFrames
var canonicalSettings = asset.MarshalImportSettings
