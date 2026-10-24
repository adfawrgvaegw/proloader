package utils

import "maps"

func MergeMaps(mapping ...map[string]any) map[string]any {
	result := make(map[string]any)
	for _, value := range mapping {
		maps.Copy(result, value)
	}
	return result
}
