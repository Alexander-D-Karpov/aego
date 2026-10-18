package schema

type regOptions struct {
	name         string
	former       []string
	version      uint32
	doc          string
	space        SpaceKind
	formerFields map[string][]string
	fieldDocs    map[string]string
	gizmos       map[string][2]string
	choices      map[string]HintSource
	bitsets      map[string]HintSource
	runtime      map[string]bool
	persist      map[string]bool
	persistAll   bool
}

type HintSource struct {
	Section string
	List    string
}

type EditorHints struct {
	DisplayName string
	Tooltip     string
	Category    string
	Step        float64
	Unit        string
	Hidden      bool
	Flags       []string
	Gizmo       string
	GizmoSpace  string
	Choices     HintSource
	Bitset      HintSource
	Runtime     bool
}

const (
	GizmoVector = "vector"
	GizmoPoint  = "point"
	GizmoRadius = "radius"
	GizmoLocal  = "local"
	GizmoWorld  = "world"
)

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

func Gizmo(field, kind string, space ...string) Option {
	return func(o *regOptions) {
		if o.gizmos == nil {
			o.gizmos = map[string][2]string{}
		}
		sp := ""
		if len(space) > 0 {
			sp = space[0]
		}
		o.gizmos[field] = [2]string{kind, sp}
	}
}

func ChoicesFrom(field, section, list string) Option {
	return func(o *regOptions) {
		if o.choices == nil {
			o.choices = map[string]HintSource{}
		}
		o.choices[field] = HintSource{Section: section, List: list}
	}
}

func Bitset(field, section, list string) Option {
	return func(o *regOptions) {
		if o.bitsets == nil {
			o.bitsets = map[string]HintSource{}
		}
		o.bitsets[field] = HintSource{Section: section, List: list}
	}
}

func Runtime(fields ...string) Option {
	return func(o *regOptions) {
		if o.runtime == nil {
			o.runtime = map[string]bool{}
		}
		for _, f := range fields {
			o.runtime[f] = true
		}
	}
}

func Persist(fields ...string) Option {
	return func(o *regOptions) {
		if len(fields) == 0 {
			o.persistAll = true
			return
		}
		if o.persist == nil {
			o.persist = map[string]bool{}
		}
		for _, f := range fields {
			o.persist[f] = true
		}
	}
}
