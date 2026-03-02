package main

import "fmt"

func (g *Generator) gen18nForValues(values []Value) {
	g.Printf("\n")

	g.Printf("var (\n")
	for i, v := range values {
		g.Printf(templateI18nDefault(i, v.originalName, v.name))
	}
	g.Printf(")\n")

}

func templateI18nDefault(idx int, constName string, i18nTranslation string) string {
	return fmt.Sprintf(`I18n_%d = &i18n.LocalizeConfig{
		DefaultMessage: &i18n.Message{
			ID:    "%s",
			Other: "%s",
		}}
		`, idx, constName, i18nTranslation)
}
