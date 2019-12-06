package uri

func Expand(path string, expansions map[string]interface{}) (string, error) {
	template, err := Parse(path)
	if err != nil {
		return "", err
	}
	values := make(map[string]interface{})
	for k, v := range expansions {
		values[k] = v
	}
	return template.Expand(values)
}