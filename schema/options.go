package schema

type regOptions struct {
	name         string
	former       []string
	version      uint32
	doc          string
	space        SpaceKind
	formerFields map[string][]string
	fieldDocs    map[string]string
}

type EditorHints struct {
	DisplayName string
	Tooltip     string
	Category    string
	Step        float64
	Unit        string
	Hidden      bool
	Flags       []string
}

type Option func(*regOptions)

func Name(full string) Option {
	return func(o *regOptions) { o.name = full }
}

func FormerName(names ...string) Option {
	return func(o *regOptions) { o.former = append(o.former, names...) }
}

func Version(v uint32) Option {
	return func(o *regOptions) { o.version = v }
}

func Doc(s string) Option {
	return func(o *regOptions) { o.doc = s }
}

func Space(k SpaceKind) Option {
	return func(o *regOptions) { o.space = k }
}

func FormerField(field string, former ...string) Option {
	return func(o *regOptions) {
		if o.formerFields == nil {
			o.formerFields = map[string][]string{}
		}
		o.formerFields[field] = append(o.formerFields[field], former...)
	}
}

func FieldDoc(field, doc string) Option {
	return func(o *regOptions) {
		if o.fieldDocs == nil {
			o.fieldDocs = map[string]string{}
		}
		o.fieldDocs[field] = doc
	}
}
