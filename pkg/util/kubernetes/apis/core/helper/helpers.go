package helper

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// IsHugePageResourceValueDivisible returns true if the resource value of storage is
// integer multiple of page size.
func IsHugePageResourceValueDivisible(name v1.ResourceName, quantity resource.Quantity) bool {
	pageSize, err := HugePageSizeFromResourceName(name)
	if err != nil {
		return false
	}

	if pageSize.Sign() <= 0 || pageSize.MilliValue()%int64(1000) != int64(0) {
		return false
	}

	return quantity.Value()%pageSize.Value() == 0
}

// HugePageSizeFromResourceName returns the page size for the specified huge page
// resource name.  If the specified input is not a valid huge page resource name
// an error is returned.
func HugePageSizeFromResourceName(name v1.ResourceName) (resource.Quantity, error) {
	if !IsHugePageResourceName(name) {
		return resource.Quantity{}, fmt.Errorf("resource name: %s is an invalid hugepage name", name)
	}
	pageSize := strings.TrimPrefix(string(name), v1.ResourceHugePagesPrefix)
	return resource.ParseQuantity(pageSize)
}

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

// IsOvercommitAllowed returns true if the resource is in the default
// namespace and is not hugepages.
func IsOvercommitAllowed(name v1.ResourceName) bool {
	return IsNativeResource(name) &&
		!IsHugePageResourceName(name)
}

// IsNativeResource returns true if the resource name is in the
// *kubernetes.io/ namespace. Partially-qualified (unprefixed) names are
// implicitly in the kubernetes.io/ namespace.
func IsNativeResource(name v1.ResourceName) bool {
	return !strings.Contains(string(name), "/") ||
		strings.Contains(string(name), v1.ResourceDefaultNamespacePrefix)
}

// IsHugePageResourceName returns true if the resource name has the huge page
// resource prefix.
func IsHugePageResourceName(name v1.ResourceName) bool {
	return strings.HasPrefix(string(name), v1.ResourceHugePagesPrefix)
}
