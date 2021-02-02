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

func equalIgnoringOrder(a1, a2 []string) bool {
	// Okay to have a slow implementation because our lists are always short.
	if len(a1) != len(a2) {
		return false
	}
	for _, valueFromA1 := range a1 {
		foundInA2 := false
		for _, valueFromA2 := range a2 {
			if valueFromA1 == valueFromA2 {
				foundInA2 = true
				break
			}
		}
		if !foundInA2 {
			return false
		}
	}
	return true
}

// Useful when you wish you could get the address of a string literal or constant.
// Works because it makes a copy of the value and returns a pointer to the copy.
func addressOf(s string) *string {
	return &s
}
