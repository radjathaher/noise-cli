package cmd

func cloneArguments(arguments map[string]any) map[string]any {
	result := make(map[string]any, len(arguments)+1)
	for key, value := range arguments {
		result[key] = value
	}
	return result
}
