package platform

type AppMetadata struct {
	Name       string
	Version    string
	Identifier string
}

var appMeta AppMetadata

func SetAppMetadata(m AppMetadata) {
	appMeta = m
}

func AppMeta() AppMetadata {
	return appMeta
}
