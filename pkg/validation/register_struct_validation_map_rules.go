package validation

func (v *validation) RegisterStructValidationMapRules(rules map[string]string, types ...interface{}) {
	v.validate.RegisterStructValidationMapRules(rules, types...)
}
