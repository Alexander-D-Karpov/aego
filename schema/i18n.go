package schema

import (
	"strings"
	"sync"

	"aego/engine/i18n"
)

// Catalog keys of the schema texts (D348): schema.<type>, schema.<type>.doc, schema.<type>.<field>, schema.<type>.<field>.doc, enum.<enum>.<value>.

type typeKeys struct{ title, doc, en string }

type fieldKeys struct{ label, doc, en string }

var (
	typeKeyCache  sync.Map
	fieldKeyCache sync.Map
	enumKeyCache  sync.Map
)

func keysOfType(td *TypeDescriptor) *typeKeys {
	if k, ok := typeKeyCache.Load(td); ok {
		return k.(*typeKeys)
	}
	k := &typeKeys{title: TypeKey(td), doc: TypeKey(td) + ".doc", en: EnglishTitle(td)}
	typeKeyCache.Store(td, k)
	return k
}

func keysOfField(td *TypeDescriptor, fd *FieldDescriptor) *fieldKeys {
	if k, ok := fieldKeyCache.Load(fd); ok {
		return k.(*fieldKeys)
	}
	k := &fieldKeys{label: FieldKey(td, fd), doc: FieldKey(td, fd) + ".doc", en: Label(fd)}
	fieldKeyCache.Store(fd, k)
	return k
}

// TypeKey is the catalog key of a type's display name.
func TypeKey(td *TypeDescriptor) string { return "schema." + td.FullName }

// FieldKey is the catalog key of a field's label.
func FieldKey(td *TypeDescriptor, fd *FieldDescriptor) string {
	return "schema." + td.FullName + "." + fd.Name
}

// EnumKey is the catalog key of an enum value's label.
func EnumKey(enum, value string) string { return "enum." + enum + "." + value }

// Title is EnglishTitle in the current language.
func Title(td *TypeDescriptor) string {
	if td.Kind == TypeScript {
		return EnglishTitle(td)
	}
	k := keysOfType(td)
	return i18n.Lookup(k.title, k.en)
}

// EnglishTitle is a type's DisplayName, else the humanized last part of its name.
func EnglishTitle(td *TypeDescriptor) string {
	if td.DisplayName != "" {
		return td.DisplayName
	}
	return Humanize(td.FullName[strings.LastIndexByte(td.FullName, '.')+1:])
}

// TypeDoc is a type's description in the current language.
func TypeDoc(td *TypeDescriptor) string {
	if td.Doc == "" || td.Kind == TypeScript {
		return td.Doc
	}
	return i18n.Lookup(keysOfType(td).doc, td.Doc)
}

// FieldLabel is Label in the current language.
func FieldLabel(td *TypeDescriptor, fd *FieldDescriptor) string {
	if td.Kind == TypeScript {
		return Label(fd)
	}
	k := keysOfField(td, fd)
	return i18n.Lookup(k.label, k.en)
}

// FieldHelp is a field's description in the current language.
func FieldHelp(td *TypeDescriptor, fd *FieldDescriptor) string {
	if fd.Doc == "" || td.Kind == TypeScript {
		return fd.Doc
	}
	return i18n.Lookup(keysOfField(td, fd).doc, fd.Doc)
}

// EnumLabel is the label of the i-th enum value of a field; English shows the value's name.
func EnumLabel(fd *FieldDescriptor, i int) string {
	if fd.EnumName == "" {
		return fd.Enum[i].Name
	}
	var keys []string
	if k, ok := enumKeyCache.Load(fd); ok {
		keys = k.([]string)
	} else {
		for _, ev := range fd.Enum {
			keys = append(keys, EnumKey(fd.EnumName, ev.Name))
		}
		enumKeyCache.Store(fd, keys)
	}
	return i18n.Lookup(keys[i], fd.Enum[i].Name)
}

// FieldTip is the field's tooltip: its description in the current language, or a tooltip set apart from it.
func FieldTip(td *TypeDescriptor, fd *FieldDescriptor) string {
	if fd.Hints.Tooltip == fd.Doc {
		return FieldHelp(td, fd)
	}
	return fd.Hints.Tooltip
}

var categoryKeys sync.Map

// CategoryLabel is a field category, such as Runtime, in the current language.
func CategoryLabel(c string) string {
	k, ok := categoryKeys.Load(c)
	if !ok {
		k = "schema.category." + c
		categoryKeys.Store(c, k)
	}
	return i18n.Lookup(k.(string), c)
}
