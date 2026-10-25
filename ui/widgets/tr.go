package widgets

// Tr translates the widgets' own words; the editor sets it to its catalog, the default keeps English (D348).
var Tr = func(key, en string) string { return en }
