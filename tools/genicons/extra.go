package main

func extraPlaceholders() map[string]string {
	warn := doc(poly(false, 12, 3, 22, 20, 2, 20), rect(11, 9, 2, 6, true), rect(11, 16, 2, 2, true))
	return map[string]string{
		"nav.back":       doc(poly(false, 4, 12, 11, 5, 11, 9.5, 20, 9.5, 20, 14.5, 11, 14.5, 11, 19)),
		"nav.forward":    doc(poly(false, 20, 12, 13, 19, 13, 14.5, 4, 14.5, 4, 9.5, 13, 9.5, 13, 5)),
		"view.split":     doc(rect(3, 4, 18, 16, false), rect(5, 6, 6, 12, true), rect(13, 6, 6, 12, true)),
		"view.minimap":   doc(rect(3, 4, 18, 16, false), rect(5, 6, 9, 12, true), rect(15.5, 6, 3.5, 4, true)),
		"view.dock":      doc(rect(3, 4, 18, 16, false), rect(5, 10, 14, 8, true)),
		"view.vectors":   doc(bar(4.5, 19.5, 14.5, 9.5, 2.6), poly(false, 20, 4, 18, 14, 10, 6)),
		"panel.problems": warn,
		"status.warning": warn,
		"help.book":      doc(poly(false, 3, 5, 11, 6.5, 11, 20, 3, 18.5), poly(false, 13, 6.5, 21, 5, 21, 18.5, 13, 20)),
		"ui.check":       doc(poly(false, 4, 12.5, 7, 9.5, 10, 12.5, 17, 5.5, 20, 8.5, 10, 18.5)),
	}
}
