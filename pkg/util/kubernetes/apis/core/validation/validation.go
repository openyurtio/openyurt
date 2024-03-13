package validation

import (
	"fmt"
	"math"
	"path"
	"path/filepath"
	"reflect"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	unversionedvalidation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/kubernetes/pkg/capabilities"
	"k8s.io/kubernetes/pkg/features"

	"github.com/openyurtio/openyurt/pkg/util/kubernetes/apis/core/helper"
)

// PodValidationOptions contains the different settings for pod validation
type PodValidationOptions struct {
	// Allow pod spec to use hugepages in downward API
	AllowDownwardAPIHugePages bool
	// Allow invalid pod-deletion-cost annotation value for backward compatibility.
	AllowInvalidPodDeletionCost bool
	// Allow pod spec to use non-integer multiple of huge page unit size
	AllowIndivisibleHugePagesValues bool
	// Allow hostProcess field to be set in windows security context
	AllowWindowsHostProcessField bool
	// Allow more DNSSearchPaths and longer DNSSearchListChars
	AllowExpandedDNSConfig bool
}

// Validates that given value is not negative.
func ValidateNonnegativeField(value int64, fldPath *field.Path) field.ErrorList {
	return apimachineryvalidation.ValidateNonnegativeField(value, fldPath)
}

// ValidatePodTemplateSpec validates the spec of a pod template
func ValidatePodTemplateSpec(spec *v1.PodTemplateSpec, fldPath *field.Path, opts PodValidationOptions) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, unversionedvalidation.ValidateLabels(spec.Labels, fldPath.Child("labels"))...)
	allErrs = append(allErrs, ValidateAnnotations(spec.Annotations, fldPath.Child("annotations"))...)
	allErrs = append(allErrs, ValidatePodSpecificAnnotations(spec.Annotations, &spec.Spec, fldPath.Child("annotations"), opts)...)
	allErrs = append(allErrs, ValidatePodSpec(&spec.Spec, nil, fldPath.Child("spec"), opts)...)
	allErrs = append(allErrs, validateSeccompAnnotationsAndFields(spec.ObjectMeta, &spec.Spec, fldPath.Child("spec"))...)

	if len(spec.Spec.EphemeralContainers) > 0 {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("spec", "ephemeralContainers"), "ephemeral containers not allowed in pod template"))
	}

	return allErrs
}

// ValidateLabels validates that a set of labels are correctly defined.
func ValidateLabels(labels map[string]string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	for k, v := range labels {
		allErrs = append(allErrs, ValidateLabelName(k, fldPath)...)
		for _, msg := range validation.IsValidLabelValue(v) {
			allErrs = append(allErrs, field.Invalid(fldPath, v, msg))
		}
	}
	return allErrs
}

// ValidateLabelName validates that the label name is correctly defined.
func ValidateLabelName(labelName string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	for _, msg := range validation.IsQualifiedName(labelName) {
		allErrs = append(allErrs, field.Invalid(fldPath, labelName, msg))
	}
	return allErrs
}

// ValidateSeccompAnnotationsAndFields iterates through all containers and ensure that when both seccompProfile and seccomp annotations exist they match.
func validateSeccompAnnotationsAndFields(objectMeta metav1.ObjectMeta, podSpec *v1.PodSpec, specPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if podSpec.SecurityContext != nil && podSpec.SecurityContext.SeccompProfile != nil {
		// If both seccomp annotations and fields are specified, the values must match.
		if annotation, found := objectMeta.Annotations[v1.SeccompPodAnnotationKey]; found {
			seccompPath := specPath.Child("securityContext").Child("seccompProfile")
			err := validateSeccompAnnotationsAndFieldsMatch(annotation, podSpec.SecurityContext.SeccompProfile, seccompPath)
			if err != nil {
				allErrs = append(allErrs, err)
			}
		}
	}

	VisitContainersWithPath(podSpec, specPath, func(c *v1.Container, cFldPath *field.Path) bool {
		var field_ *v1.SeccompProfile
		if c.SecurityContext != nil {
			field_ = c.SecurityContext.SeccompProfile
		}

		if field_ == nil {
			return true
		}

		key := v1.SeccompContainerAnnotationKeyPrefix + c.Name
		if annotation, found := objectMeta.Annotations[key]; found {
			seccompPath := cFldPath.Child("securityContext").Child("seccompProfile")
			err := validateSeccompAnnotationsAndFieldsMatch(annotation, field_, seccompPath)
			if err != nil {
				allErrs = append(allErrs, err)
			}
		}
		return true
	})

	return allErrs
}

func validateSeccompAnnotationsAndFieldsMatch(annotationValue string, seccompField *v1.SeccompProfile, fldPath *field.Path) *field.Error {
	if seccompField == nil {
		return nil
	}

	switch seccompField.Type {
	case v1.SeccompProfileTypeUnconfined:
		if annotationValue != v1.SeccompProfileNameUnconfined {
			return field.Forbidden(fldPath.Child("type"), "seccomp type in annotation and field must match")
		}

	case v1.SeccompProfileTypeRuntimeDefault:
		if annotationValue != v1.SeccompProfileRuntimeDefault && annotationValue != v1.DeprecatedSeccompProfileDockerDefault {
			return field.Forbidden(fldPath.Child("type"), "seccomp type in annotation and field must match")
		}

	case v1.SeccompProfileTypeLocalhost:
		if !strings.HasPrefix(annotationValue, v1.SeccompLocalhostProfileNamePrefix) {
			return field.Forbidden(fldPath.Child("type"), "seccomp type in annotation and field must match")
		} else if seccompField.LocalhostProfile == nil || strings.TrimPrefix(annotationValue, v1.SeccompLocalhostProfileNamePrefix) != *seccompField.LocalhostProfile {
			return field.Forbidden(fldPath.Child("localhostProfile"), "seccomp profile in annotation and field must match")
		}
	}

	return nil
}

// ValidateAnnotations validates that a set of annotations are correctly defined.
func ValidateAnnotations(annotations map[string]string, fldPath *field.Path) field.ErrorList {
	return apimachineryvalidation.ValidateAnnotations(annotations, fldPath)
}

// ValidatePodSpec tests that the specified PodSpec has valid data.
// This includes checking formatting and uniqueness.  It also canonicalizes the
// structure by setting default values and implementing any backwards-compatibility
// tricks.
// The pod metadata is needed to validate generic ephemeral volumes. It is optional
// and should be left empty unless the spec is from a real pod object.
func ValidatePodSpec(spec *v1.PodSpec, podMeta *metav1.ObjectMeta, fldPath *field.Path, opts PodValidationOptions) field.ErrorList {
	allErrs := field.ErrorList{}

	var gracePeriod int64
	if spec.TerminationGracePeriodSeconds != nil {
		// this could happen in tests
		gracePeriod = *spec.TerminationGracePeriodSeconds
	}

	vols, vErrs := ValidateVolumes(spec.Volumes, podMeta, fldPath.Child("volumes"), opts)
	allErrs = append(allErrs, vErrs...)
	podClaimNames := gatherPodResourceClaimNames(spec.ResourceClaims)
	allErrs = append(allErrs, validatePodResourceClaims(podMeta, spec.ResourceClaims, fldPath.Child("resourceClaims"))...)
	allErrs = append(allErrs, validateContainers(spec.Containers, vols, podClaimNames, gracePeriod, fldPath.Child("containers"), opts, &spec.RestartPolicy)...)
	allErrs = append(allErrs, validateInitContainers(spec.InitContainers, spec.Containers, vols, podClaimNames, gracePeriod, fldPath.Child("initContainers"), opts, &spec.RestartPolicy)...)
	allErrs = append(allErrs, validateEphemeralContainers(spec.EphemeralContainers, spec.Containers, spec.InitContainers, vols, podClaimNames, fldPath.Child("ephemeralContainers"), opts, &spec.RestartPolicy)...)
	allErrs = append(allErrs, validatePodHostNetworkDeps(spec, fldPath, opts)...)
	allErrs = append(allErrs, validateRestartPolicy(&spec.RestartPolicy, fldPath.Child("restartPolicy"))...)
	allErrs = append(allErrs, validateDNSPolicy(&spec.DNSPolicy, fldPath.Child("dnsPolicy"))...)
	allErrs = append(allErrs, unversionedvalidation.ValidateLabels(spec.NodeSelector, fldPath.Child("nodeSelector"))...)
	allErrs = append(allErrs, validatePodSpecSecurityContext(spec.SecurityContext, spec, fldPath, fldPath.Child("securityContext"), opts)...)
	allErrs = append(allErrs, validateImagePullSecrets(spec.ImagePullSecrets, fldPath.Child("imagePullSecrets"))...)
	allErrs = append(allErrs, validateAffinity(spec.Affinity, opts, fldPath.Child("affinity"))...)
	allErrs = append(allErrs, validatePodDNSConfig(spec.DNSConfig, &spec.DNSPolicy, fldPath.Child("dnsConfig"), opts)...)
	allErrs = append(allErrs, validateReadinessGates(spec.ReadinessGates, fldPath.Child("readinessGates"))...)
	allErrs = append(allErrs, validateSchedulingGates(spec.SchedulingGates, fldPath.Child("schedulingGates"))...)
	allErrs = append(allErrs, validateTopologySpreadConstraints(spec.TopologySpreadConstraints, fldPath.Child("topologySpreadConstraints"), opts)...)
	allErrs = append(allErrs, validateWindowsHostProcessPod(spec, fldPath)...)
	allErrs = append(allErrs, validateHostUsers(spec, fldPath)...)
	if len(spec.ServiceAccountName) > 0 {
		for _, msg := range ValidateServiceAccountName(spec.ServiceAccountName, false) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("serviceAccountName"), spec.ServiceAccountName, msg))
		}
	}

	if len(spec.NodeName) > 0 {
		for _, msg := range ValidateNodeName(spec.NodeName, false) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("nodeName"), spec.NodeName, msg))
		}
	}

	if spec.ActiveDeadlineSeconds != nil {
		value := *spec.ActiveDeadlineSeconds
		if value < 1 || value > math.MaxInt32 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("activeDeadlineSeconds"), value, validation.InclusiveRangeError(1, math.MaxInt32)))
		}
	}

	if len(spec.Hostname) > 0 {
		allErrs = append(allErrs, ValidateDNS1123Label(spec.Hostname, fldPath.Child("hostname"))...)
	}

	if len(spec.Subdomain) > 0 {
		allErrs = append(allErrs, ValidateDNS1123Label(spec.Subdomain, fldPath.Child("subdomain"))...)
	}

	if len(spec.Tolerations) > 0 {
		allErrs = append(allErrs, ValidateTolerations(spec.Tolerations, fldPath.Child("tolerations"))...)
	}

	if len(spec.HostAliases) > 0 {
		allErrs = append(allErrs, ValidateHostAliases(spec.HostAliases, fldPath.Child("hostAliases"))...)
	}

	if len(spec.PriorityClassName) > 0 {
		for _, msg := range ValidatePriorityClassName(spec.PriorityClassName, false) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("priorityClassName"), spec.PriorityClassName, msg))
		}
	}

	if spec.RuntimeClassName != nil {
		allErrs = append(allErrs, ValidateRuntimeClassName(*spec.RuntimeClassName, fldPath.Child("runtimeClassName"))...)
	}

	if spec.PreemptionPolicy != nil {
		allErrs = append(allErrs, ValidatePreemptionPolicy(spec.PreemptionPolicy, fldPath.Child("preemptionPolicy"))...)
	}

	if spec.Overhead != nil {
		allErrs = append(allErrs, validateOverhead(spec.Overhead, fldPath.Child("overhead"), opts)...)
	}

	if spec.OS != nil {
		osErrs := validateOS(spec, fldPath.Child("os"), opts)
		switch {
		case len(osErrs) > 0:
			allErrs = append(allErrs, osErrs...)
		case spec.OS.Name == v1.Linux:
			allErrs = append(allErrs, validateLinux(spec, fldPath)...)
		case spec.OS.Name == v1.Windows:
			allErrs = append(allErrs, validateWindows(spec, fldPath)...)
		}
	}
	return allErrs
}

// Validates resource requirement spec.
func ValidateResourceRequirements(requirements *v1.ResourceRequirements, podClaimNames sets.Set[string], fldPath *field.Path, opts PodValidationOptions) field.ErrorList {
	allErrs := field.ErrorList{}
	limPath := fldPath.Child("limits")
	reqPath := fldPath.Child("requests")
	limContainsCPUOrMemory := false
	reqContainsCPUOrMemory := false
	limContainsHugePages := false
	reqContainsHugePages := false
	supportedQoSComputeResources := sets.New(v1.ResourceCPU, v1.ResourceMemory)
	for resourceName, quantity := range requirements.Limits {

		fldPath := limPath.Key(string(resourceName))
		// Validate resource name.
		allErrs = append(allErrs, validateContainerResourceName(resourceName, fldPath)...)

		// Validate resource quantity.
		allErrs = append(allErrs, ValidateResourceQuantityValue(resourceName, quantity, fldPath)...)

		if helper.IsHugePageResourceName(resourceName) {
			limContainsHugePages = true
			if err := validateResourceQuantityHugePageValue(resourceName, quantity, opts); err != nil {
				allErrs = append(allErrs, field.Invalid(fldPath, quantity.String(), err.Error()))
			}
		}

		if supportedQoSComputeResources.Has(resourceName) {
			limContainsCPUOrMemory = true
		}
	}
	for resourceName, quantity := range requirements.Requests {
		fldPath := reqPath.Key(string(resourceName))
		// Validate resource name.
		allErrs = append(allErrs, validateContainerResourceName(resourceName, fldPath)...)
		// Validate resource quantity.
		allErrs = append(allErrs, ValidateResourceQuantityValue(resourceName, quantity, fldPath)...)

		// Check that request <= limit.
		limitQuantity, exists := requirements.Limits[resourceName]
		if exists {
			// For non overcommitable resources, not only requests can't exceed limits, they also can't be lower, i.e. must be equal.
			if quantity.Cmp(limitQuantity) != 0 && !helper.IsOvercommitAllowed(resourceName) {
				allErrs = append(allErrs, field.Invalid(reqPath, quantity.String(), fmt.Sprintf("must be equal to %s limit of %s", resourceName, limitQuantity.String())))
			} else if quantity.Cmp(limitQuantity) > 0 {
				allErrs = append(allErrs, field.Invalid(reqPath, quantity.String(), fmt.Sprintf("must be less than or equal to %s limit of %s", resourceName, limitQuantity.String())))
			}
		} else if !helper.IsOvercommitAllowed(resourceName) {
			allErrs = append(allErrs, field.Required(limPath, "Limit must be set for non overcommitable resources"))
		}
		if helper.IsHugePageResourceName(resourceName) {
			reqContainsHugePages = true
			if err := validateResourceQuantityHugePageValue(resourceName, quantity, opts); err != nil {
				allErrs = append(allErrs, field.Invalid(fldPath, quantity.String(), err.Error()))
			}
		}
		if supportedQoSComputeResources.Has(resourceName) {
			reqContainsCPUOrMemory = true
		}

	}
	if !limContainsCPUOrMemory && !reqContainsCPUOrMemory && (reqContainsHugePages || limContainsHugePages) {
		allErrs = append(allErrs, field.Forbidden(fldPath, "HugePages require cpu or memory"))
	}

	allErrs = append(allErrs, validateResourceClaimNames(requirements.Claims, podClaimNames, fldPath.Child("claims"))...)

	return allErrs
}

// validateResourceClaimNames checks that the names in
// ResourceRequirements.Claims have a corresponding entry in
// PodSpec.ResourceClaims.
func validateResourceClaimNames(claims []v1.ResourceClaim, podClaimNames sets.Set[string], fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	names := sets.Set[string]{}
	for i, claim := range claims {
		name := claim.Name
		if name == "" {
			allErrs = append(allErrs, field.Required(fldPath.Index(i), ""))
		} else {
			if names.Has(name) {
				allErrs = append(allErrs, field.Duplicate(fldPath.Index(i), name))
			} else {
				names.Insert(name)
			}
			if !podClaimNames.Has(name) {
				// field.NotFound doesn't accept an
				// explanation. Adding one here is more
				// user-friendly.
				error := field.NotFound(fldPath.Index(i), name)
				error.Detail = "must be one of the names in pod.spec.resourceClaims"
				if len(podClaimNames) == 0 {
					error.Detail += " which is empty"
				} else {
					error.Detail += ": " + strings.Join(sets.List(podClaimNames), ", ")
				}
				allErrs = append(allErrs, error)
			}
		}
	}
	return allErrs
}

func validateResourceQuantityHugePageValue(name v1.ResourceName, quantity resource.Quantity, opts PodValidationOptions) error {
	if !helper.IsHugePageResourceName(name) {
		return nil
	}

	if !opts.AllowIndivisibleHugePagesValues && !helper.IsHugePageResourceValueDivisible(name, quantity) {
		return fmt.Errorf("%s is not positive integer multiple of %s", quantity.String(), name)
	}

	return nil
}

// ValidateResourceQuantityValue enforces that specified quantity is valid for specified resource
func ValidateResourceQuantityValue(resource v1.ResourceName, value resource.Quantity, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, ValidateNonnegativeQuantity(value, fldPath)...)
	if helper.IsIntegerResourceName(resource) {
		if value.MilliValue()%int64(1000) != int64(0) {
			allErrs = append(allErrs, field.Invalid(fldPath, value, isNotIntegerErrorMsg))
		}
	}
	return allErrs
}

// Validate container resource name
// Refer to docs/design/resources.md for more details.
func validateContainerResourceName(value v1.ResourceName, fldPath *field.Path) field.ErrorList {
	allErrs := validateResourceName(value, fldPath)

	if len(strings.Split(string(value), "/")) == 1 {
		if !helper.IsStandardContainerResourceName(value) {
			return append(allErrs, field.Invalid(fldPath, value, "must be a standard resource for containers"))
		}
	} else if !helper.IsNativeResource(value) {
		if !helper.IsExtendedResourceName(value) {
			return append(allErrs, field.Invalid(fldPath, value, "doesn't follow extended resource name standard"))
		}
	}
	return allErrs
}

// validateOverhead can be used to check whether the given Overhead is valid.
func validateOverhead(overhead v1.ResourceList, fldPath *field.Path, opts PodValidationOptions) field.ErrorList {
	// reuse the ResourceRequirements validation logic
	return ValidateResourceRequirements(&v1.ResourceRequirements{Limits: overhead}, nil, fldPath, opts)
}

func ValidatePreemptionPolicy(preemptionPolicy *v1.PreemptionPolicy, fldPath *field.Path) field.ErrorList {
	allErrors := field.ErrorList{}
	switch *preemptionPolicy {
	case v1.PreemptLowerPriority, v1.PreemptNever:
	case "":
		allErrors = append(allErrors, field.Required(fldPath, ""))
	default:
		validValues := []v1.PreemptionPolicy{v1.PreemptLowerPriority, v1.PreemptNever}
		allErrors = append(allErrors, field.NotSupported(fldPath, preemptionPolicy, validValues))
	}
	return allErrors
}

func ValidateHostAliases(hostAliases []v1.HostAlias, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	for i, hostAlias := range hostAliases {
		allErrs = append(allErrs, validation.IsValidIP(fldPath.Index(i).Child("ip"), hostAlias.IP)...)
		for j, hostname := range hostAlias.Hostnames {
			allErrs = append(allErrs, ValidateDNS1123Subdomain(hostname, fldPath.Index(i).Child("hostnames").Index(j))...)
		}
	}
	return allErrs
}

// ValidateNameFunc validates that the provided name is valid for a given resource type.
// Not all resources have the same validation rules for names. Prefix is true
// if the name will have a value appended to it.  If the name is not valid,
// this returns a list of descriptions of individual characteristics of the
// value that were not valid.  Otherwise this returns an empty list or nil.
type ValidateNameFunc apimachineryvalidation.ValidateNameFunc

// ValidatePodName can be used to check whether the given pod name is valid.
// Prefix indicates this name will be used as part of generation, in which case
// trailing dashes are allowed.
var ValidatePodName = apimachineryvalidation.NameIsDNSSubdomain

// ValidateReplicationControllerName can be used to check whether the given replication
// controller name is valid.
// Prefix indicates this name will be used as part of generation, in which case
// trailing dashes are allowed.
var ValidateReplicationControllerName = apimachineryvalidation.NameIsDNSSubdomain

// ValidateServiceName can be used to check whether the given service name is valid.
// Prefix indicates this name will be used as part of generation, in which case
// trailing dashes are allowed.
var ValidateServiceName = apimachineryvalidation.NameIsDNS1035Label

// ValidateNodeName can be used to check whether the given node name is valid.
// Prefix indicates this name will be used as part of generation, in which case
// trailing dashes are allowed.
var ValidateNodeName = apimachineryvalidation.NameIsDNSSubdomain

// ValidateNamespaceName can be used to check whether the given namespace name is valid.
// Prefix indicates this name will be used as part of generation, in which case
// trailing dashes are allowed.
var ValidateNamespaceName = apimachineryvalidation.ValidateNamespaceName

// ValidateLimitRangeName can be used to check whether the given limit range name is valid.
// Prefix indicates this name will be used as part of generation, in which case
// trailing dashes are allowed.
var ValidateLimitRangeName = apimachineryvalidation.NameIsDNSSubdomain

// ValidateResourceQuotaName can be used to check whether the given
// resource quota name is valid.
// Prefix indicates this name will be used as part of generation, in which case
// trailing dashes are allowed.
var ValidateResourceQuotaName = apimachineryvalidation.NameIsDNSSubdomain

// ValidateSecretName can be used to check whether the given secret name is valid.
// Prefix indicates this name will be used as part of generation, in which case
// trailing dashes are allowed.
var ValidateSecretName = apimachineryvalidation.NameIsDNSSubdomain

// ValidateServiceAccountName can be used to check whether the given service account name is valid.
// Prefix indicates this name will be used as part of generation, in which case
// trailing dashes are allowed.
var ValidateServiceAccountName = apimachineryvalidation.ValidateServiceAccountName

// ValidateEndpointsName can be used to check whether the given endpoints name is valid.
// Prefix indicates this name will be used as part of generation, in which case
// trailing dashes are allowed.
var ValidateEndpointsName = apimachineryvalidation.NameIsDNSSubdomain

// ValidateClassName can be used to check whether the given class name is valid.
// It is defined here to avoid import cycle between pkg/apis/storage/validation
// (where it should be) and this file.
var ValidateClassName = apimachineryvalidation.NameIsDNSSubdomain

// ValidatePriorityClassName can be used to check whether the given priority
// class name is valid.
var ValidatePriorityClassName = apimachineryvalidation.NameIsDNSSubdomain

// ValidateResourceClaimName can be used to check whether the given
// name for a ResourceClaim is valid.
var ValidateResourceClaimName = apimachineryvalidation.NameIsDNSSubdomain

// ValidateResourceClaimTemplateName can be used to check whether the given
// name for a ResourceClaimTemplate is valid.
var ValidateResourceClaimTemplateName = apimachineryvalidation.NameIsDNSSubdomain

// ValidateRuntimeClassName can be used to check whether the given RuntimeClass name is valid.
// Prefix indicates this name will be used as part of generation, in which case
// trailing dashes are allowed.
func ValidateRuntimeClassName(name string, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	for _, msg := range apimachineryvalidation.NameIsDNSSubdomain(name, false) {
		allErrs = append(allErrs, field.Invalid(fldPath, name, msg))
	}
	return allErrs
}

func validateHostUsers(spec *v1.PodSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Only make the following checks if hostUsers is false (otherwise, the container uses the
	// same userns as the host, and so there isn't anything to check).
	if spec.SecurityContext == nil || spec.SecurityContext.HostUsers == nil || *spec.SecurityContext.HostUsers {
		return allErrs
	}

	// We decided to restrict the usage of userns with other host namespaces:
	// 	https://github.com/kubernetes/kubernetes/pull/111090#discussion_r935994282
	// The tl;dr is: you can easily run into permission issues that seem unexpected, we don't
	// know of any good use case and we can always enable them later.

	// Note we already validated above spec.SecurityContext is not nil.
	if spec.SecurityContext.HostNetwork {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("hostNetwork"), "when `pod.Spec.HostUsers` is false"))
	}
	if spec.SecurityContext.HostPID {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("HostPID"), "when `pod.Spec.HostUsers` is false"))
	}
	if spec.SecurityContext.HostIPC {
		allErrs = append(allErrs, field.Forbidden(fldPath.Child("HostIPC"), "when `pod.Spec.HostUsers` is false"))
	}

	return allErrs
}

func validateSchedulingGates(schedulingGates []v1.PodSchedulingGate, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	// There should be no duplicates in the list of scheduling gates.
	seen := sets.Set[string]{}
	for i, schedulingGate := range schedulingGates {
		allErrs = append(allErrs, ValidateQualifiedName(schedulingGate.Name, fldPath.Index(i))...)
		if seen.Has(schedulingGate.Name) {
			allErrs = append(allErrs, field.Duplicate(fldPath.Index(i), schedulingGate.Name))
		}
		seen.Insert(schedulingGate.Name)
	}
	return allErrs
}

// validatePodHostNetworkDeps checks fields which depend on whether HostNetwork is
// true or not.  It should be called on all PodSpecs, but opts can change what
// is enforce.  E.g. opts.ResourceIsPod should only be set when called in the
// context of a Pod, and not on PodSpecs which are embedded in other resources
// (e.g. Deployments).
func validatePodHostNetworkDeps(spec *v1.PodSpec, fldPath *field.Path, opts PodValidationOptions) field.ErrorList {
	// For <reasons> we keep `.HostNetwork` in .SecurityContext on the internal
	// version of Pod.
	hostNetwork := false
	if spec.SecurityContext != nil {
		hostNetwork = spec.SecurityContext.HostNetwork
	}

	allErrors := field.ErrorList{}

	if hostNetwork {
		fldPath := fldPath.Child("containers")
		for i, container := range spec.Containers {
			portsPath := fldPath.Index(i).Child("ports")
			for i, port := range container.Ports {
				idxPath := portsPath.Index(i)
				// At this point, we know that HostNetwork is true. If this
				// PodSpec is in a Pod (opts.ResourceIsPod), then HostPort must
				// be the same value as ContainerPort. If this PodSpec is in
				// some other resource (e.g. Deployment) we allow 0 (i.e.
				// unspecified) because it will be defaulted when the Pod is
				// ultimately created, but we do not allow any other values.
				if hp, cp := port.HostPort, port.ContainerPort; (opts.ResourceIsPod || hp != 0) && hp != cp {
					allErrors = append(allErrors, field.Invalid(idxPath.Child("hostPort"), port.HostPort, "must match `containerPort` when `hostNetwork` is true"))
				}
			}
		}
	}
	return allErrors
}

func validatePodResourceClaims(podMeta *metav1.ObjectMeta, claims []v1.PodResourceClaim, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	podClaimNames := sets.New[string]()
	for i, claim := range claims {
		allErrs = append(allErrs, validatePodResourceClaim(podMeta, claim, &podClaimNames, fldPath.Index(i))...)
	}
	return allErrs
}

func validatePodResourceClaim(podMeta *metav1.ObjectMeta, claim v1.PodResourceClaim, podClaimNames *sets.Set[string], fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	if claim.Name == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("name"), ""))
	} else if podClaimNames.Has(claim.Name) {
		allErrs = append(allErrs, field.Duplicate(fldPath.Child("name"), claim.Name))
	} else {
		nameErrs := ValidateDNS1123Label(claim.Name, fldPath.Child("name"))
		if len(nameErrs) > 0 {
			allErrs = append(allErrs, nameErrs...)
		} else if podMeta != nil && claim.Source.ResourceClaimTemplateName != nil {
			claimName := podMeta.Name + "-" + claim.Name
			for _, detail := range ValidateResourceClaimName(claimName, false) {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("name"), claimName, "final ResourceClaim name: "+detail))
			}
		}
		podClaimNames.Insert(claim.Name)
	}
	allErrs = append(allErrs, validatePodResourceClaimSource(claim.Source, fldPath.Child("source"))...)

	return allErrs
}

func validatePodResourceClaimSource(claimSource v1.ClaimSource, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	if claimSource.ResourceClaimName != nil && claimSource.ResourceClaimTemplateName != nil {
		allErrs = append(allErrs, field.Invalid(fldPath, claimSource, "at most one of `resourceClaimName` or `resourceClaimTemplateName` may be specified"))
	}
	if claimSource.ResourceClaimName == nil && claimSource.ResourceClaimTemplateName == nil {
		allErrs = append(allErrs, field.Invalid(fldPath, claimSource, "must specify one of: `resourceClaimName`, `resourceClaimTemplateName`"))
	}
	if claimSource.ResourceClaimName != nil {
		for _, detail := range ValidateResourceClaimName(*claimSource.ResourceClaimName, false) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("resourceClaimName"), *claimSource.ResourceClaimName, detail))
		}
	}
	if claimSource.ResourceClaimTemplateName != nil {
		for _, detail := range ValidateResourceClaimTemplateName(*claimSource.ResourceClaimTemplateName, false) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("resourceClaimTemplateName"), *claimSource.ResourceClaimTemplateName, detail))
		}
	}
	return allErrs
}

// gatherPodResourceClaimNames returns a set of all non-empty
// PodResourceClaim.Name values. Validation that those names are valid is
// handled by validatePodResourceClaims.
func gatherPodResourceClaimNames(claims []v1.PodResourceClaim) sets.Set[string] {
	podClaimNames := sets.Set[string]{}
	for _, claim := range claims {
		if claim.Name != "" {
			podClaimNames.Insert(claim.Name)
		}
	}
	return podClaimNames
}

func validateWindowsHostProcessPod(podSpec *v1.PodSpec, fieldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Keep track of container and hostProcess container count for validate
	containerCount := 0
	hostProcessContainerCount := 0

	var podHostProcess *bool
	if podSpec.SecurityContext != nil && podSpec.SecurityContext.WindowsOptions != nil {
		podHostProcess = podSpec.SecurityContext.WindowsOptions.HostProcess
	}

	hostNetwork := false
	if podSpec.SecurityContext != nil {
		hostNetwork = podSpec.SecurityContext.HostNetwork
	}

	podshelper.VisitContainersWithPath(podSpec, fieldPath, func(c *v1.Container, cFieldPath *field.Path) bool {
		containerCount++

		var containerHostProcess *bool = nil
		if c.SecurityContext != nil && c.SecurityContext.WindowsOptions != nil {
			containerHostProcess = c.SecurityContext.WindowsOptions.HostProcess
		}

		if podHostProcess != nil && containerHostProcess != nil && *podHostProcess != *containerHostProcess {
			errMsg := fmt.Sprintf("pod hostProcess value must be identical if both are specified, was %v", *podHostProcess)
			allErrs = append(allErrs, field.Invalid(cFieldPath.Child("securityContext", "windowsOptions", "hostProcess"), *containerHostProcess, errMsg))
		}

		switch {
		case containerHostProcess != nil && *containerHostProcess:
			// Container explicitly sets hostProcess=true
			hostProcessContainerCount++
		case containerHostProcess == nil && podHostProcess != nil && *podHostProcess:
			// Container inherits hostProcess=true from pod settings
			hostProcessContainerCount++
		}

		return true
	})

	if hostProcessContainerCount > 0 {
		// At present, if a Windows Pods contains any HostProcess containers than all containers must be
		// HostProcess containers (explicitly set or inherited).
		if hostProcessContainerCount != containerCount {
			errMsg := "If pod contains any hostProcess containers then all containers must be HostProcess containers"
			allErrs = append(allErrs, field.Invalid(fieldPath, "", errMsg))
		}

		// At present Windows Pods which contain HostProcess containers must also set HostNetwork.
		if !hostNetwork {
			errMsg := "hostNetwork must be true if pod contains any hostProcess containers"
			allErrs = append(allErrs, field.Invalid(fieldPath.Child("hostNetwork"), hostNetwork, errMsg))
		}

		if !capabilities.Get().AllowPrivileged {
			errMsg := "hostProcess containers are disallowed by cluster policy"
			allErrs = append(allErrs, field.Forbidden(fieldPath, errMsg))
		}
	}

	return allErrs
}

// validateTopologySpreadConstraints validates given TopologySpreadConstraints.
func validateTopologySpreadConstraints(constraints []v1.TopologySpreadConstraint, fldPath *field.Path, opts PodValidationOptions) field.ErrorList {
	allErrs := field.ErrorList{}

	for i, constraint := range constraints {
		subFldPath := fldPath.Index(i)
		if err := ValidateMaxSkew(subFldPath.Child("maxSkew"), constraint.MaxSkew); err != nil {
			allErrs = append(allErrs, err)
		}
		if err := ValidateTopologyKey(subFldPath.Child("topologyKey"), constraint.TopologyKey); err != nil {
			allErrs = append(allErrs, err)
		}
		if err := ValidateWhenUnsatisfiable(subFldPath.Child("whenUnsatisfiable"), constraint.WhenUnsatisfiable); err != nil {
			allErrs = append(allErrs, err)
		}
		// tuple {topologyKey, whenUnsatisfiable} denotes one kind of spread constraint
		if err := ValidateSpreadConstraintNotRepeat(subFldPath.Child("{topologyKey, whenUnsatisfiable}"), constraint, constraints[i+1:]); err != nil {
			allErrs = append(allErrs, err)
		}
		allErrs = append(allErrs, validateMinDomains(subFldPath.Child("minDomains"), constraint.MinDomains, constraint.WhenUnsatisfiable)...)
		if err := validateNodeInclusionPolicy(subFldPath.Child("nodeAffinityPolicy"), constraint.NodeAffinityPolicy); err != nil {
			allErrs = append(allErrs, err)
		}
		if err := validateNodeInclusionPolicy(subFldPath.Child("nodeTaintsPolicy"), constraint.NodeTaintsPolicy); err != nil {
			allErrs = append(allErrs, err)
		}
		allErrs = append(allErrs, validateMatchLabelKeysInTopologySpread(subFldPath.Child("matchLabelKeys"), constraint.MatchLabelKeys, constraint.LabelSelector)...)
		if !opts.AllowInvalidTopologySpreadConstraintLabelSelector {
			allErrs = append(allErrs, unversionedvalidation.ValidateLabelSelector(constraint.LabelSelector, unversionedvalidation.LabelSelectorValidationOptions{AllowInvalidLabelValueInSelector: false}, subFldPath.Child("labelSelector"))...)
		}
	}

	return allErrs
}

func validateReadinessGates(readinessGates []v1.PodReadinessGate, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	for i, value := range readinessGates {
		allErrs = append(allErrs, ValidateQualifiedName(string(value.ConditionType), fldPath.Index(i).Child("conditionType"))...)
	}
	return allErrs
}

func validatePodDNSConfig(dnsConfig *v1.PodDNSConfig, dnsPolicy *v1.DNSPolicy, fldPath *field.Path, opts PodValidationOptions) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate DNSNone case. Must provide at least one DNS name server.
	if dnsPolicy != nil && *dnsPolicy == v1.DNSNone {
		if dnsConfig == nil {
			return append(allErrs, field.Required(fldPath, fmt.Sprintf("must provide `dnsConfig` when `dnsPolicy` is %s", v1.DNSNone)))
		}
		if len(dnsConfig.Nameservers) == 0 {
			return append(allErrs, field.Required(fldPath.Child("nameservers"), fmt.Sprintf("must provide at least one DNS nameserver when `dnsPolicy` is %s", v1.DNSNone)))
		}
	}

	if dnsConfig != nil {
		// Validate nameservers.
		if len(dnsConfig.Nameservers) > MaxDNSNameservers {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("nameservers"), dnsConfig.Nameservers, fmt.Sprintf("must not have more than %v nameservers", MaxDNSNameservers)))
		}
		for i, ns := range dnsConfig.Nameservers {
			allErrs = append(allErrs, validation.IsValidIP(fldPath.Child("nameservers").Index(i), ns)...)
		}
		// Validate searches.
		if len(dnsConfig.Searches) > MaxDNSSearchPaths {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("searches"), dnsConfig.Searches, fmt.Sprintf("must not have more than %v search paths", MaxDNSSearchPaths)))
		}
		// Include the space between search paths.
		if len(strings.Join(dnsConfig.Searches, " ")) > MaxDNSSearchListChars {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("searches"), dnsConfig.Searches, fmt.Sprintf("must not have more than %v characters (including spaces) in the search list", MaxDNSSearchListChars)))
		}
		for i, search := range dnsConfig.Searches {
			// it is fine to have a trailing dot
			search = strings.TrimSuffix(search, ".")
			allErrs = append(allErrs, ValidateDNS1123Subdomain(search, fldPath.Child("searches").Index(i))...)
		}
		// Validate options.
		for i, option := range dnsConfig.Options {
			if len(option.Name) == 0 {
				allErrs = append(allErrs, field.Required(fldPath.Child("options").Index(i), "must not be empty"))
			}
		}
	}
	return allErrs
}

// validateAffinity checks if given affinities are valid
func validateAffinity(affinity *v1.Affinity, opts PodValidationOptions, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if affinity != nil {
		if affinity.NodeAffinity != nil {
			allErrs = append(allErrs, validateNodeAffinity(affinity.NodeAffinity, fldPath.Child("nodeAffinity"))...)
		}
		if affinity.PodAffinity != nil {
			allErrs = append(allErrs, validatePodAffinity(affinity.PodAffinity, opts.AllowInvalidLabelValueInSelector, fldPath.Child("podAffinity"))...)
		}
		if affinity.PodAntiAffinity != nil {
			allErrs = append(allErrs, validatePodAntiAffinity(affinity.PodAntiAffinity, opts.AllowInvalidLabelValueInSelector, fldPath.Child("podAntiAffinity"))...)
		}
	}

	return allErrs
}

// validateImagePullSecrets checks to make sure the pull secrets are well
// formed.  Right now, we only expect name to be set (it's the only field).  If
// this ever changes and someone decides to set those fields, we'd like to
// know.
func validateImagePullSecrets(imagePullSecrets []v1.LocalObjectReference, fldPath *field.Path) field.ErrorList {
	allErrors := field.ErrorList{}
	for i, currPullSecret := range imagePullSecrets {
		idxPath := fldPath.Index(i)
		strippedRef := v1.LocalObjectReference{Name: currPullSecret.Name}
		if !reflect.DeepEqual(strippedRef, currPullSecret) {
			allErrors = append(allErrors, field.Invalid(idxPath, currPullSecret, "only name may be set"))
		}
	}
	return allErrors
}

// validatePodSpecSecurityContext verifies the SecurityContext of a PodSpec,
// whether that is defined in a Pod or in an embedded PodSpec (e.g. a
// Deployment's pod template).
func validatePodSpecSecurityContext(securityContext *v1.PodSecurityContext, spec *v1.PodSpec, specPath, fldPath *field.Path, opts PodValidationOptions) field.ErrorList {
	allErrs := field.ErrorList{}

	if securityContext != nil {
		if securityContext.FSGroup != nil {
			for _, msg := range validation.IsValidGroupID(*securityContext.FSGroup) {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("fsGroup"), *(securityContext.FSGroup), msg))
			}
		}
		if securityContext.RunAsUser != nil {
			for _, msg := range validation.IsValidUserID(*securityContext.RunAsUser) {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("runAsUser"), *(securityContext.RunAsUser), msg))
			}
		}
		if securityContext.RunAsGroup != nil {
			for _, msg := range validation.IsValidGroupID(*securityContext.RunAsGroup) {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("runAsGroup"), *(securityContext.RunAsGroup), msg))
			}
		}
		for g, gid := range securityContext.SupplementalGroups {
			for _, msg := range validation.IsValidGroupID(gid) {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("supplementalGroups").Index(g), gid, msg))
			}
		}
		if securityContext.ShareProcessNamespace != nil && securityContext.HostPID && *securityContext.ShareProcessNamespace {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("shareProcessNamespace"), *securityContext.ShareProcessNamespace, "ShareProcessNamespace and HostPID cannot both be enabled"))
		}

		if len(securityContext.Sysctls) != 0 {
			allErrs = append(allErrs, validateSysctls(securityContext, fldPath.Child("sysctls"), opts)...)
		}

		if securityContext.FSGroupChangePolicy != nil {
			allErrs = append(allErrs, validateFSGroupChangePolicy(securityContext.FSGroupChangePolicy, fldPath.Child("fsGroupChangePolicy"))...)
		}

		allErrs = append(allErrs, validateSeccompProfileField(securityContext.SeccompProfile, fldPath.Child("seccompProfile"))...)
		allErrs = append(allErrs, validateWindowsSecurityContextOptions(securityContext.WindowsOptions, fldPath.Child("windowsOptions"))...)
	}

	return allErrs
}

func validateDNSPolicy(dnsPolicy *v1.DNSPolicy, fldPath *field.Path) field.ErrorList {
	allErrors := field.ErrorList{}
	switch *dnsPolicy {
	case v1.DNSClusterFirstWithHostNet, v1.DNSClusterFirst, v1.DNSDefault, v1.DNSNone:
	case "":
		allErrors = append(allErrors, field.Required(fldPath, ""))
	default:
		validValues := []v1.DNSPolicy{v1.DNSClusterFirstWithHostNet, v1.DNSClusterFirst, v1.DNSDefault, v1.DNSNone}
		allErrors = append(allErrors, field.NotSupported(fldPath, dnsPolicy, validValues))
	}
	return allErrors
}

func validateRestartPolicy(restartPolicy *v1.RestartPolicy, fldPath *field.Path) field.ErrorList {
	allErrors := field.ErrorList{}
	switch *restartPolicy {
	case v1.RestartPolicyAlways, v1.RestartPolicyOnFailure, v1.RestartPolicyNever:
		break
	case "":
		allErrors = append(allErrors, field.Required(fldPath, ""))
	default:
		validValues := []v1.RestartPolicy{v1.RestartPolicyAlways, v1.RestartPolicyOnFailure, v1.RestartPolicyNever}
		allErrors = append(allErrors, field.NotSupported(fldPath, *restartPolicy, validValues))
	}

	return allErrors
}

// validateEphemeralContainers is called by pod spec and template validation to validate the list of ephemeral containers.
// Note that this is called for pod template even though ephemeral containers aren't allowed in pod templates.
func validateEphemeralContainers(ephemeralContainers []v1.EphemeralContainer, containers, initContainers []v1.Container, volumes map[string]v1.VolumeSource, podClaimNames sets.Set[string], fldPath *field.Path, opts PodValidationOptions, podRestartPolicy *v1.RestartPolicy) field.ErrorList {
	var allErrs field.ErrorList

	if len(ephemeralContainers) == 0 {
		return allErrs
	}

	otherNames, allNames := sets.Set[string]{}, sets.Set[string]{}
	for _, c := range containers {
		otherNames.Insert(c.Name)
		allNames.Insert(c.Name)
	}
	for _, c := range initContainers {
		otherNames.Insert(c.Name)
		allNames.Insert(c.Name)
	}

	for i, ec := range ephemeralContainers {
		idxPath := fldPath.Index(i)

		c := (*v1.Container)(&ec.EphemeralContainerCommon)
		allErrs = append(allErrs, validateContainerCommon(c, volumes, podClaimNames, idxPath, opts, podRestartPolicy)...)
		// Ephemeral containers don't need looser constraints for pod templates, so it's convenient to apply both validations
		// here where we've already converted EphemeralContainerCommon to Container.
		allErrs = append(allErrs, validateContainerOnlyForPod(c, idxPath)...)

		// Ephemeral containers must have a name unique across all container types.
		if allNames.Has(ec.Name) {
			allErrs = append(allErrs, field.Duplicate(idxPath.Child("name"), ec.Name))
		} else {
			allNames.Insert(ec.Name)
		}

		// The target container name must exist and be non-ephemeral.
		if ec.TargetContainerName != "" && !otherNames.Has(ec.TargetContainerName) {
			allErrs = append(allErrs, field.NotFound(idxPath.Child("targetContainerName"), ec.TargetContainerName))
		}

		// Ephemeral containers should not be relied upon for fundamental pod services, so fields such as
		// Lifecycle, probes, resources and ports should be disallowed. This is implemented as a list
		// of allowed fields so that new fields will be given consideration prior to inclusion in ephemeral containers.
		allErrs = append(allErrs, validateFieldAllowList(ec.EphemeralContainerCommon, allowedEphemeralContainerFields, "cannot be set for an Ephemeral Container", idxPath)...)

		// VolumeMount subpaths have the potential to leak resources since they're implemented with bind mounts
		// that aren't cleaned up until the pod exits. Since they also imply that the container is being used
		// as part of the workload, they're disallowed entirely.
		for i, vm := range ec.VolumeMounts {
			if vm.SubPath != "" {
				allErrs = append(allErrs, field.Forbidden(idxPath.Child("volumeMounts").Index(i).Child("subPath"), "cannot be set for an Ephemeral Container"))
			}
			if vm.SubPathExpr != "" {
				allErrs = append(allErrs, field.Forbidden(idxPath.Child("volumeMounts").Index(i).Child("subPathExpr"), "cannot be set for an Ephemeral Container"))
			}
		}
	}

	return allErrs
}

// validateInitContainers is called by pod spec and template validation to validate the list of init containers
func validateInitContainers(containers []v1.Container, regularContainers []v1.Container, volumes map[string]v1.VolumeSource, podClaimNames sets.Set[string], gracePeriod int64, fldPath *field.Path, opts PodValidationOptions, podRestartPolicy *v1.RestartPolicy) field.ErrorList {
	var allErrs field.ErrorList

	allNames := sets.Set[string]{}
	for _, ctr := range regularContainers {
		allNames.Insert(ctr.Name)
	}
	for i, ctr := range containers {
		idxPath := fldPath.Index(i)

		// Apply the validation common to all container types
		allErrs = append(allErrs, validateContainerCommon(&ctr, volumes, podClaimNames, idxPath, opts, podRestartPolicy)...)

		restartAlways := false
		// Apply the validation specific to init containers
		if ctr.RestartPolicy != nil {
			allErrs = append(allErrs, validateInitContainerRestartPolicy(ctr.RestartPolicy, idxPath.Child("restartPolicy"))...)
			restartAlways = *ctr.RestartPolicy == v1.ContainerRestartPolicyAlways
		}

		// Names must be unique within regular and init containers. Collisions with ephemeral containers
		// will be detected by validateEphemeralContainers().
		if allNames.Has(ctr.Name) {
			allErrs = append(allErrs, field.Duplicate(idxPath.Child("name"), ctr.Name))
		} else if len(ctr.Name) > 0 {
			allNames.Insert(ctr.Name)
		}

		// Check for port conflicts in init containers individually since init containers run one-by-one.
		allErrs = append(allErrs, checkHostPortConflicts([]v1.Container{ctr}, fldPath)...)

		switch {
		case restartAlways:
			if ctr.Lifecycle != nil {
				allErrs = append(allErrs, validateLifecycle(ctr.Lifecycle, gracePeriod, idxPath.Child("lifecycle"))...)
			}
			allErrs = append(allErrs, validateLivenessProbe(ctr.LivenessProbe, gracePeriod, idxPath.Child("livenessProbe"))...)
			allErrs = append(allErrs, validateReadinessProbe(ctr.ReadinessProbe, gracePeriod, idxPath.Child("readinessProbe"))...)
			allErrs = append(allErrs, validateStartupProbe(ctr.StartupProbe, gracePeriod, idxPath.Child("startupProbe"))...)

		default:
			// These fields are disallowed for init containers.
			if ctr.Lifecycle != nil {
				allErrs = append(allErrs, field.Forbidden(idxPath.Child("lifecycle"), "may not be set for init containers without restartPolicy=Always"))
			}
			if ctr.LivenessProbe != nil {
				allErrs = append(allErrs, field.Forbidden(idxPath.Child("livenessProbe"), "may not be set for init containers without restartPolicy=Always"))
			}
			if ctr.ReadinessProbe != nil {
				allErrs = append(allErrs, field.Forbidden(idxPath.Child("readinessProbe"), "may not be set for init containers without restartPolicy=Always"))
			}
			if ctr.StartupProbe != nil {
				allErrs = append(allErrs, field.Forbidden(idxPath.Child("startupProbe"), "may not be set for init containers without restartPolicy=Always"))
			}
		}

		if len(ctr.ResizePolicy) > 0 {
			allErrs = append(allErrs, field.Invalid(idxPath.Child("resizePolicy"), ctr.ResizePolicy, "must not be set for init containers"))
		}
	}

	return allErrs
}

func GetVolumeMountMap(mounts []v1.VolumeMount) map[string]string {
	volmounts := make(map[string]string)

	for _, mnt := range mounts {
		volmounts[mnt.Name] = mnt.MountPath
	}

	return volmounts
}

func GetVolumeDeviceMap(devices []v1.VolumeDevice) map[string]string {
	volDevices := make(map[string]string)

	for _, dev := range devices {
		volDevices[dev.Name] = dev.DevicePath
	}

	return volDevices
}

func ValidateDNS1123Label(value string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	for _, msg := range validation.IsDNS1123Label(value) {
		allErrs = append(allErrs, field.Invalid(fldPath, value, msg))
	}
	return allErrs
}

// validateContainers is called by pod spec and template validation to validate the list of regular containers.
func validateContainers(containers []v1.Container, volumes map[string]v1.VolumeSource, podClaimNames sets.Set[string], gracePeriod int64, fldPath *field.Path, opts PodValidationOptions, podRestartPolicy *v1.RestartPolicy) field.ErrorList {
	allErrs := field.ErrorList{}

	if len(containers) == 0 {
		return append(allErrs, field.Required(fldPath, ""))
	}

	allNames := sets.Set[string]{}
	for i, ctr := range containers {
		path := fldPath.Index(i)

		// Apply validation common to all containers
		allErrs = append(allErrs, validateContainerCommon(&ctr, volumes, podClaimNames, path, opts, podRestartPolicy)...)

		// Container names must be unique within the list of regular containers.
		// Collisions with init or ephemeral container names will be detected by the init or ephemeral
		// container validation to prevent duplicate error messages.
		if allNames.Has(ctr.Name) {
			allErrs = append(allErrs, field.Duplicate(path.Child("name"), ctr.Name))
		} else {
			allNames.Insert(ctr.Name)
		}

		// These fields are allowed for regular containers and restartable init
		// containers.
		// Regular init container and ephemeral container validation will return
		// field.Forbidden() for these paths.
		if ctr.Lifecycle != nil {
			allErrs = append(allErrs, validateLifecycle(ctr.Lifecycle, gracePeriod, path.Child("lifecycle"))...)
		}
		allErrs = append(allErrs, validateLivenessProbe(ctr.LivenessProbe, gracePeriod, path.Child("livenessProbe"))...)
		allErrs = append(allErrs, validateReadinessProbe(ctr.ReadinessProbe, gracePeriod, path.Child("readinessProbe"))...)
		allErrs = append(allErrs, validateStartupProbe(ctr.StartupProbe, gracePeriod, path.Child("startupProbe"))...)

		// These fields are disallowed for regular containers
		if ctr.RestartPolicy != nil {
			allErrs = append(allErrs, field.Forbidden(path.Child("restartPolicy"), "may not be set for non-init containers"))
		}
	}

	// Port conflicts are checked across all containers
	allErrs = append(allErrs, checkHostPortConflicts(containers, fldPath)...)

	return allErrs
}

func ValidateVolumeMounts(mounts []v1.VolumeMount, voldevices map[string]string, volumes map[string]v1.VolumeSource, container *v1.Container, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	mountpoints := sets.NewString()

	for i, mnt := range mounts {
		idxPath := fldPath.Index(i)
		if len(mnt.Name) == 0 {
			allErrs = append(allErrs, field.Required(idxPath.Child("name"), ""))
		}
		if !IsMatchedVolume(mnt.Name, volumes) {
			allErrs = append(allErrs, field.NotFound(idxPath.Child("name"), mnt.Name))
		}
		if len(mnt.MountPath) == 0 {
			allErrs = append(allErrs, field.Required(idxPath.Child("mountPath"), ""))
		}
		if mountpoints.Has(mnt.MountPath) {
			allErrs = append(allErrs, field.Invalid(idxPath.Child("mountPath"), mnt.MountPath, "must be unique"))
		}
		mountpoints.Insert(mnt.MountPath)

		// check for overlap with VolumeDevice
		if mountNameAlreadyExists(mnt.Name, voldevices) {
			allErrs = append(allErrs, field.Invalid(idxPath.Child("name"), mnt.Name, "must not already exist in volumeDevices"))
		}
		if mountPathAlreadyExists(mnt.MountPath, voldevices) {
			allErrs = append(allErrs, field.Invalid(idxPath.Child("mountPath"), mnt.MountPath, "must not already exist as a path in volumeDevices"))
		}

		if len(mnt.SubPath) > 0 {
			allErrs = append(allErrs, validateLocalDescendingPath(mnt.SubPath, fldPath.Child("subPath"))...)
		}

		if len(mnt.SubPathExpr) > 0 {
			if len(mnt.SubPath) > 0 {
				allErrs = append(allErrs, field.Invalid(idxPath.Child("subPathExpr"), mnt.SubPathExpr, "subPathExpr and subPath are mutually exclusive"))
			}

			allErrs = append(allErrs, validateLocalDescendingPath(mnt.SubPathExpr, fldPath.Child("subPathExpr"))...)
		}

		if mnt.MountPropagation != nil {
			allErrs = append(allErrs, validateMountPropagation(mnt.MountPropagation, container, fldPath.Child("mountPropagation"))...)
		}
	}
	return allErrs
}

// validateMountPropagation verifies that MountPropagation field is valid and
// allowed for given container.
func validateMountPropagation(mountPropagation *v1.MountPropagationMode, container *v1.Container, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if mountPropagation == nil {
		return allErrs
	}

	supportedMountPropagations := sets.NewString(string(v1.MountPropagationBidirectional), string(v1.MountPropagationHostToContainer), string(v1.MountPropagationNone))
	if !supportedMountPropagations.Has(string(*mountPropagation)) {
		allErrs = append(allErrs, field.NotSupported(fldPath, *mountPropagation, supportedMountPropagations.List()))
	}

	if container == nil {
		// The container is not available yet.
		// Stop validation now, Pod validation will refuse final
		// Pods with Bidirectional propagation in non-privileged containers.
		return allErrs
	}

	privileged := container.SecurityContext != nil && container.SecurityContext.Privileged != nil && *container.SecurityContext.Privileged
	if *mountPropagation == v1.MountPropagationBidirectional && !privileged {
		allErrs = append(allErrs, field.Forbidden(fldPath, "Bidirectional mount propagation is available only to privileged containers"))
	}
	return allErrs
}

func IsMatchedVolume(name string, volumes map[string]v1.VolumeSource) bool {
	if _, ok := volumes[name]; ok {
		return true
	}
	return false
}

func mountNameAlreadyExists(name string, devices map[string]string) bool {
	if _, ok := devices[name]; ok {
		return true
	}
	return false
}

func mountPathAlreadyExists(mountPath string, devices map[string]string) bool {
	for _, devPath := range devices {
		if mountPath == devPath {
			return true
		}
	}

	return false
}

func ValidateVolumeDevices(devices []v1.VolumeDevice, volmounts map[string]string, volumes map[string]v1.VolumeSource, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	devicepath := sets.NewString()
	devicename := sets.NewString()

	for i, dev := range devices {
		idxPath := fldPath.Index(i)
		devName := dev.Name
		devPath := dev.DevicePath
		didMatch, isPVC := isMatchedDevice(devName, volumes)
		if len(devName) == 0 {
			allErrs = append(allErrs, field.Required(idxPath.Child("name"), ""))
		}
		if devicename.Has(devName) {
			allErrs = append(allErrs, field.Invalid(idxPath.Child("name"), devName, "must be unique"))
		}
		// Must be PersistentVolumeClaim volume source
		if didMatch && !isPVC {
			allErrs = append(allErrs, field.Invalid(idxPath.Child("name"), devName, "can only use volume source type of PersistentVolumeClaim for block mode"))
		}
		if !didMatch {
			allErrs = append(allErrs, field.NotFound(idxPath.Child("name"), devName))
		}
		if len(devPath) == 0 {
			allErrs = append(allErrs, field.Required(idxPath.Child("devicePath"), ""))
		}
		if devicepath.Has(devPath) {
			allErrs = append(allErrs, field.Invalid(idxPath.Child("devicePath"), devPath, "must be unique"))
		}
		if len(devPath) > 0 && len(validatePathNoBacksteps(devPath, fldPath.Child("devicePath"))) > 0 {
			allErrs = append(allErrs, field.Invalid(idxPath.Child("devicePath"), devPath, "can not contain backsteps ('..')"))
		} else {
			devicepath.Insert(devPath)
		}
		// check for overlap with VolumeMount
		if deviceNameAlreadyExists(devName, volmounts) {
			allErrs = append(allErrs, field.Invalid(idxPath.Child("name"), devName, "must not already exist in volumeMounts"))
		}
		if devicePathAlreadyExists(devPath, volmounts) {
			allErrs = append(allErrs, field.Invalid(idxPath.Child("devicePath"), devPath, "must not already exist as a path in volumeMounts"))
		}
		if len(devName) > 0 {
			devicename.Insert(devName)
		}
	}
	return allErrs
}

func isMatchedDevice(name string, volumes map[string]v1.VolumeSource) (bool, bool) {
	if source, ok := volumes[name]; ok {
		if source.PersistentVolumeClaim != nil {
			return true, true
		}
		return true, false
	}
	return false, false
}

func deviceNameAlreadyExists(name string, mounts map[string]string) bool {
	if _, ok := mounts[name]; ok {
		return true
	}
	return false
}
func devicePathAlreadyExists(devicePath string, mounts map[string]string) bool {
	for _, mountPath := range mounts {
		if mountPath == devicePath {
			return true
		}
	}

	return false
}

var supportedPullPolicies = sets.NewString(string(v1.PullAlways), string(v1.PullIfNotPresent), string(v1.PullNever))

func validatePullPolicy(policy v1.PullPolicy, fldPath *field.Path) field.ErrorList {
	allErrors := field.ErrorList{}

	switch policy {
	case v1.PullAlways, v1.PullIfNotPresent, v1.PullNever:
		break
	case "":
		allErrors = append(allErrors, field.Required(fldPath, ""))
	default:
		allErrors = append(allErrors, field.NotSupported(fldPath, policy, supportedPullPolicies.List()))
	}

	return allErrors
}
func ValidateVolumes(volumes []v1.Volume, podMeta *metav1.ObjectMeta, fldPath *field.Path, opts PodValidationOptions) (map[string]v1.VolumeSource, field.ErrorList) {
	allErrs := field.ErrorList{}

	//allNames := sets.String{}
	allCreatedPVCs := sets.String{}
	// Determine which PVCs will be created for this pod. We need
	// the exact name of the pod for this. Without it, this sanity
	// check has to be skipped.
	if podMeta != nil && podMeta.Name != "" {
		for _, vol := range volumes {
			if vol.VolumeSource.Ephemeral != nil {
				allCreatedPVCs.Insert(podMeta.Name + "-" + vol.Name)
			}
		}
	}
	vols := make(map[string]v1.VolumeSource)
	for i, vol := range volumes {
		idxPath := fldPath.Index(i)
		//namePath := idxPath.Child("name")
		//el := validateVolumeSource(&vol.VolumeSource, idxPath, vol.Name, podMeta, opts)
		//if len(vol.Name) == 0 {
		//	el = append(el, field.Required(namePath, ""))
		//} else {
		//	el = append(el, ValidateDNS1123Label(vol.Name, namePath)...)
		//}
		//if allNames.Has(vol.Name) {
		//	el = append(el, field.Duplicate(namePath, vol.Name))
		//}
		//if len(el) == 0 {
		//	allNames.Insert(vol.Name)
		//	vols[vol.Name] = vol.VolumeSource
		//} else {
		//	allErrs = append(allErrs, el...)
		//}
		// A PersistentVolumeClaimSource should not reference a created PVC. That doesn't
		// make sense.
		if vol.PersistentVolumeClaim != nil && allCreatedPVCs.Has(vol.PersistentVolumeClaim.ClaimName) {
			allErrs = append(allErrs, field.Invalid(idxPath.Child("persistentVolumeClaim").Child("claimName"), vol.PersistentVolumeClaim.ClaimName,
				"must not reference a PVC that gets created for an ephemeral volume"))
		}
	}

	return vols, allErrs
}

// ValidateTolerationsInPodAnnotations tests that the serialized tolerations in Pod.Annotations has valid data
func ValidateTolerationsInPodAnnotations(annotations map[string]string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	tolerations, err := helper.GetTolerationsFromPodAnnotations(annotations)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath, v1.TolerationsAnnotationKey, err.Error()))
		return allErrs
	}

	if len(tolerations) > 0 {
		allErrs = append(allErrs, ValidateTolerations(tolerations, fldPath.Child(v1.TolerationsAnnotationKey))...)
	}

	return allErrs
}

// ValidateTolerations tests if given tolerations have valid data.
func ValidateTolerations(tolerations []v1.Toleration, fldPath *field.Path) field.ErrorList {
	allErrors := field.ErrorList{}
	for i, toleration := range tolerations {
		idxPath := fldPath.Index(i)
		// validate the toleration key
		if len(toleration.Key) > 0 {
			allErrors = append(allErrors, unversionedvalidation.ValidateLabelName(toleration.Key, idxPath.Child("key"))...)
		}

		// empty toleration key with Exists operator and empty value means match all taints
		if len(toleration.Key) == 0 && toleration.Operator != v1.TolerationOpExists {
			allErrors = append(allErrors, field.Invalid(idxPath.Child("operator"), toleration.Operator,
				"operator must be Exists when `key` is empty, which means \"match all values and all keys\""))
		}

		if toleration.TolerationSeconds != nil && toleration.Effect != v1.TaintEffectNoExecute {
			allErrors = append(allErrors, field.Invalid(idxPath.Child("effect"), toleration.Effect,
				"effect must be 'NoExecute' when `tolerationSeconds` is set"))
		}

		// validate toleration operator and value
		switch toleration.Operator {
		// empty operator means Equal
		case v1.TolerationOpEqual, "":
			if errs := validation.IsValidLabelValue(toleration.Value); len(errs) != 0 {
				allErrors = append(allErrors, field.Invalid(idxPath.Child("operator"), toleration.Value, strings.Join(errs, ";")))
			}
		case v1.TolerationOpExists:
			if len(toleration.Value) > 0 {
				allErrors = append(allErrors, field.Invalid(idxPath.Child("operator"), toleration, "value must be empty when `operator` is 'Exists'"))
			}
		default:
			validValues := []string{string(v1.TolerationOpEqual), string(v1.TolerationOpExists)}
			allErrors = append(allErrors, field.NotSupported(idxPath.Child("operator"), toleration.Operator, validValues))
		}

		// validate toleration effect, empty toleration effect means match all taint effects
		if len(toleration.Effect) > 0 {
			allErrors = append(allErrors, validateTaintEffect(&toleration.Effect, true, idxPath.Child("effect"))...)
		}
	}
	return allErrors
}

func validateTaintEffect(effect *v1.TaintEffect, allowEmpty bool, fldPath *field.Path) field.ErrorList {
	if !allowEmpty && len(*effect) == 0 {
		return field.ErrorList{field.Required(fldPath, "")}
	}

	allErrors := field.ErrorList{}
	switch *effect {
	// TODO: Replace next line with subsequent commented-out line when implement TaintEffectNoScheduleNoAdmit.
	case v1.TaintEffectNoSchedule, v1.TaintEffectPreferNoSchedule, v1.TaintEffectNoExecute:
		// case v1.TaintEffectNoSchedule, v1.TaintEffectPreferNoSchedule, v1.TaintEffectNoScheduleNoAdmit, v1.TaintEffectNoExecute:
	default:
		validValues := []string{
			string(v1.TaintEffectNoSchedule),
			string(v1.TaintEffectPreferNoSchedule),
			string(v1.TaintEffectNoExecute),
			// TODO: Uncomment this block when implement TaintEffectNoScheduleNoAdmit.
			// string(v1.TaintEffectNoScheduleNoAdmit),
		}
		allErrors = append(allErrors, field.NotSupported(fldPath, *effect, validValues))
	}
	return allErrors
}

func ValidateSeccompPodAnnotations(annotations map[string]string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if p, exists := annotations[v1.SeccompPodAnnotationKey]; exists {
		allErrs = append(allErrs, ValidateSeccompProfile(p, fldPath.Child(v1.SeccompPodAnnotationKey))...)
	}
	for k, p := range annotations {
		if strings.HasPrefix(k, v1.SeccompContainerAnnotationKeyPrefix) {
			allErrs = append(allErrs, ValidateSeccompProfile(p, fldPath.Child(k))...)
		}
	}

	return allErrs
}

func ValidateSeccompProfile(p string, fldPath *field.Path) field.ErrorList {
	if p == v1.SeccompProfileRuntimeDefault || p == v1.DeprecatedSeccompProfileDockerDefault {
		return nil
	}
	if p == v1.SeccompProfileNameUnconfined {
		return nil
	}
	if strings.HasPrefix(p, v1.SeccompLocalhostProfileNamePrefix) {
		return validateLocalDescendingPath(strings.TrimPrefix(p, v1.SeccompLocalhostProfileNamePrefix), fldPath)
	}
	return field.ErrorList{field.Invalid(fldPath, p, "must be a valid seccomp profile")}
}

// This validate will make sure targetPath:
// 1. is not abs path
// 2. does not have any element which is ".."
func validateLocalDescendingPath(targetPath string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if path.IsAbs(targetPath) {
		allErrs = append(allErrs, field.Invalid(fldPath, targetPath, "must be a relative path"))
	}

	allErrs = append(allErrs, validatePathNoBacksteps(targetPath, fldPath)...)

	return allErrs
}

func ValidateAppArmorPodAnnotations(annotations map[string]string, spec *v1.PodSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	for k, p := range annotations {
		if !strings.HasPrefix(k, v1.AppArmorBetaContainerAnnotationKeyPrefix) {
			continue
		}
		containerName := strings.TrimPrefix(k, v1.AppArmorBetaContainerAnnotationKeyPrefix)
		if !podSpecHasContainer(spec, containerName) {
			allErrs = append(allErrs, field.Invalid(fldPath.Key(k), containerName, "container not found"))
		}

		if err := ValidateProfileFormat(p); err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Key(k), p, err.Error()))
		}
	}

	return allErrs
}

// ValidateProfileFormat checks the format of the profile.
func ValidateProfileFormat(profile string) error {
	if profile == "" || profile == v1.AppArmorBetaProfileRuntimeDefault || profile == v1.AppArmorBetaProfileNameUnconfined {
		return nil
	}
	if !strings.HasPrefix(profile, v1.AppArmorBetaProfileNamePrefix) {
		return fmt.Errorf("invalid AppArmor profile name: %q", profile)
	}
	return nil
}

// ContainerVisitorWithPath is called with each container and the field.Path to that container,
// and returns true if visiting should continue.
type ContainerVisitorWithPath func(container *v1.Container, path *field.Path) bool

// VisitContainersWithPath invokes the visitor function with a pointer to the spec
// of every container in the given pod spec and the field.Path to that container.
// If visitor returns false, visiting is short-circuited. VisitContainersWithPath returns true if visiting completes,
// false if visiting was short-circuited.
func VisitContainersWithPath(podSpec *v1.PodSpec, specPath *field.Path, visitor ContainerVisitorWithPath) bool {
	fldPath := specPath.Child("initContainers")
	for i := range podSpec.InitContainers {
		if !visitor(&podSpec.InitContainers[i], fldPath.Index(i)) {
			return false
		}
	}
	fldPath = specPath.Child("containers")
	for i := range podSpec.Containers {
		if !visitor(&podSpec.Containers[i], fldPath.Index(i)) {
			return false
		}
	}
	if utilfeature.DefaultFeatureGate.Enabled(features.EphemeralContainers) {
		fldPath = specPath.Child("ephemeralContainers")
		for i := range podSpec.EphemeralContainers {
			if !visitor((*v1.Container)(&podSpec.EphemeralContainers[i].EphemeralContainerCommon), fldPath.Index(i)) {
				return false
			}
		}
	}
	return true
}

func podSpecHasContainer(spec *v1.PodSpec, containerName string) bool {
	var hasContainer bool
	VisitContainersWithPath(spec, field.NewPath("spec"), func(c *v1.Container, _ *field.Path) bool {
		if c.Name == containerName {
			hasContainer = true
			return false
		}
		return true
	})
	return hasContainer
}

// validatePathNoBacksteps makes sure the targetPath does not have any `..` path elements when split
//
// This assumes the OS of the apiserver and the nodes are the same. The same check should be done
// on the node to ensure there are no backsteps.
func validatePathNoBacksteps(targetPath string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	parts := strings.Split(filepath.ToSlash(targetPath), "/")
	for _, item := range parts {
		if item == ".." {
			allErrs = append(allErrs, field.Invalid(fldPath, targetPath, "must not contain '..'"))
			break // even for `../../..`, one error is sufficient to make the point
		}
	}
	return allErrs
}

func ValidateReadOnlyPersistentDisks(volumes []v1.Volume, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	for i := range volumes {
		vol := &volumes[i]
		idxPath := fldPath.Index(i)
		if vol.GCEPersistentDisk != nil {
			if !vol.GCEPersistentDisk.ReadOnly {
				allErrs = append(allErrs, field.Invalid(idxPath.Child("gcePersistentDisk", "readOnly"), false, "must be true for replicated pods > 1; GCE PD can only be mounted on multiple machines if it is read-only"))
			}
		}
		// TODO: What to do for AWS?  It doesn't support replicas
	}
	return allErrs
}

func ValidatePodSpecificAnnotations(annotations map[string]string, spec *v1.PodSpec, fldPath *field.Path, opts PodValidationOptions) field.ErrorList {
	allErrs := field.ErrorList{}

	if value, isMirror := annotations[v1.MirrorPodAnnotationKey]; isMirror {
		if len(spec.NodeName) == 0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Key(v1.MirrorPodAnnotationKey), value, "must set spec.nodeName if mirror pod annotation is set"))
		}
	}

	if annotations[v1.TolerationsAnnotationKey] != "" {
		allErrs = append(allErrs, ValidateTolerationsInPodAnnotations(annotations, fldPath)...)
	}

	if !opts.AllowInvalidPodDeletionCost {
		if _, err := helper.GetDeletionCostFromPodAnnotations(annotations); err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Key(v1.PodDeletionCost), annotations[v1.PodDeletionCost], "must be a 32bit integer"))
		}
	}

	allErrs = append(allErrs, ValidateSeccompPodAnnotations(annotations, fldPath)...)
	allErrs = append(allErrs, ValidateAppArmorPodAnnotations(annotations, spec, fldPath)...)

	return allErrs
}
