package trackeractivity

func uniqueValuesFromMapOfSlices(theMap map[string][]string) []string {
	allValues := []string{}
	for _, values := range theMap {
		for _, value := range values {
			if !contains(value, allValues) {
				allValues = append(allValues, value)
			}
		}
	}
	return allValues
}

func contains(needle string, haystack []string) bool {
	for _, a := range haystack {
		if a == needle {
			return true
		}
	}
	return false
}

func removeElements(removeFrom []string, removeThese []string) []string {
	result := []string{}
	for _, s := range removeFrom {
		if !contains(s, removeThese) {
			result = append(result, s)
		}
	}
	return result
}
