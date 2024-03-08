package helper

import (
	"encoding/json"
	"fmt"
	"strconv"

	v1 "k8s.io/api/core/v1"
)

// GetDeletionCostFromPodAnnotations returns the integer value of pod-deletion-cost. Returns 0
// if not set or the value is invalid.
func GetDeletionCostFromPodAnnotations(annotations map[string]string) (int32, error) {
	if value, exist := annotations[v1.PodDeletionCost]; exist {
		// values that start with plus sign (e.g, "+10") or leading zeros (e.g., "008") are not valid.
		if !validFirstDigit(value) {
			return 0, fmt.Errorf("invalid value %q", value)
		}

		i, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			// make sure we default to 0 on error.
			return 0, err
		}
		return int32(i), nil
	}
	return 0, nil
}

func validFirstDigit(str string) bool {
	if len(str) == 0 {
		return false
	}
	return str[0] == '-' || (str[0] == '0' && str == "0") || (str[0] >= '1' && str[0] <= '9')
}

// GetTolerationsFromPodAnnotations gets the json serialized tolerations data from Pod.Annotations
// and converts it to the []Toleration type in v1.
func GetTolerationsFromPodAnnotations(annotations map[string]string) ([]v1.Toleration, error) {
	var tolerations []v1.Toleration
	if len(annotations) > 0 && annotations[v1.TolerationsAnnotationKey] != "" {
		err := json.Unmarshal([]byte(annotations[v1.TolerationsAnnotationKey]), &tolerations)
		if err != nil {
			return tolerations, err
		}
	}
	return tolerations, nil
}
