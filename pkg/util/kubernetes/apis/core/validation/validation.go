package validation

import (
	"fmt"
	"math"
	"path"
	"path/filepath"
	"strings"

	v1 "k8s.io/api/core/v1"
	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	unversionedvalidation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
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
	//allErrs = append(allErrs, ValidatePodSpec(&spec.Spec, nil, fldPath.Child("spec"), opts)...)
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

	vols, vErrs := ValidateVolumes(spec.Volumes, podMeta, fldPath.Child("volumes"), opts)
	allErrs = append(allErrs, vErrs...)
	allErrs = append(allErrs, validateContainers(spec.Containers, false, vols, fldPath.Child("containers"), opts)...)
	//allErrs = append(allErrs, validateInitContainers(spec.InitContainers, spec.Containers, vols, fldPath.Child("initContainers"), opts)...)
	//allErrs = append(allErrs, validateEphemeralContainers(spec.EphemeralContainers, spec.Containers, spec.InitContainers, vols, fldPath.Child("ephemeralContainers"), opts)...)
	//allErrs = append(allErrs, validateRestartPolicy(&spec.RestartPolicy, fldPath.Child("restartPolicy"))...)
	//allErrs = append(allErrs, validateDNSPolicy(&spec.DNSPolicy, fldPath.Child("dnsPolicy"))...)
	//allErrs = append(allErrs, unversionedvalidation.ValidateLabels(spec.NodeSelector, fldPath.Child("nodeSelector"))...)
	//allErrs = append(allErrs, ValidatePodSecurityContext(spec.SecurityContext, spec, fldPath, fldPath.Child("securityContext"))...)
	//allErrs = append(allErrs, validateImagePullSecrets(spec.ImagePullSecrets, fldPath.Child("imagePullSecrets"))...)
	//allErrs = append(allErrs, validateAffinity(spec.Affinity, fldPath.Child("affinity"))...)
	//allErrs = append(allErrs, validatePodDNSConfig(spec.DNSConfig, &spec.DNSPolicy, fldPath.Child("dnsConfig"), opts)...)
	//allErrs = append(allErrs, validateReadinessGates(spec.ReadinessGates, fldPath.Child("readinessGates"))...)
	//allErrs = append(allErrs, validateTopologySpreadConstraints(spec.TopologySpreadConstraints, fldPath.Child("topologySpreadConstraints"))...)
	//allErrs = append(allErrs, validateWindowsHostProcessPod(spec, fldPath, opts)...)
	//if len(spec.ServiceAccountName) > 0 {
	//	for _, msg := range ValidateServiceAccountName(spec.ServiceAccountName, false) {
	//		allErrs = append(allErrs, field.Invalid(fldPath.Child("serviceAccountName"), spec.ServiceAccountName, msg))
	//	}
	//}
	//
	//if len(spec.NodeName) > 0 {
	//	for _, msg := range ValidateNodeName(spec.NodeName, false) {
	//		allErrs = append(allErrs, field.Invalid(fldPath.Child("nodeName"), spec.NodeName, msg))
	//	}
	//}

	if spec.ActiveDeadlineSeconds != nil {
		value := *spec.ActiveDeadlineSeconds
		if value < 1 || value > math.MaxInt32 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("activeDeadlineSeconds"), value, validation.InclusiveRangeError(1, math.MaxInt32)))
		}
	}

	//if len(spec.Hostname) > 0 {
	//	allErrs = append(allErrs, ValidateDNS1123Label(spec.Hostname, fldPath.Child("hostname"))...)
	//}
	//
	//if len(spec.Subdomain) > 0 {
	//	allErrs = append(allErrs, ValidateDNS1123Label(spec.Subdomain, fldPath.Child("subdomain"))...)
	//}

	if len(spec.Tolerations) > 0 {
		allErrs = append(allErrs, ValidateTolerations(spec.Tolerations, fldPath.Child("tolerations"))...)
	}

	//if len(spec.HostAliases) > 0 {
	//	allErrs = append(allErrs, ValidateHostAliases(spec.HostAliases, fldPath.Child("hostAliases"))...)
	//}
	//
	//if len(spec.PriorityClassName) > 0 {
	//	for _, msg := range ValidatePriorityClassName(spec.PriorityClassName, false) {
	//		allErrs = append(allErrs, field.Invalid(fldPath.Child("priorityClassName"), spec.PriorityClassName, msg))
	//	}
	//}
	//
	//if spec.RuntimeClassName != nil {
	//	allErrs = append(allErrs, ValidateRuntimeClassName(*spec.RuntimeClassName, fldPath.Child("runtimeClassName"))...)
	//}
	//
	//if spec.PreemptionPolicy != nil {
	//	allErrs = append(allErrs, ValidatePreemptionPolicy(spec.PreemptionPolicy, fldPath.Child("preemptionPolicy"))...)
	//}
	//
	//if spec.Overhead != nil {
	//	allErrs = append(allErrs, validateOverhead(spec.Overhead, fldPath.Child("overhead"), opts)...)
	//}

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

func validateContainers(containers []v1.Container, isInitContainers bool, volumes map[string]v1.VolumeSource, fldPath *field.Path, opts PodValidationOptions) field.ErrorList {
	allErrs := field.ErrorList{}

	if len(containers) == 0 {
		return append(allErrs, field.Required(fldPath, ""))
	}

	allNames := sets.String{}
	for i, ctr := range containers {
		idxPath := fldPath.Index(i)
		namePath := idxPath.Child("name")
		volMounts := GetVolumeMountMap(ctr.VolumeMounts)
		volDevices := GetVolumeDeviceMap(ctr.VolumeDevices)

		if len(ctr.Name) == 0 {
			allErrs = append(allErrs, field.Required(namePath, ""))
		} else {
			allErrs = append(allErrs, ValidateDNS1123Label(ctr.Name, namePath)...)
		}
		if allNames.Has(ctr.Name) {
			allErrs = append(allErrs, field.Duplicate(namePath, ctr.Name))
		} else {
			allNames.Insert(ctr.Name)
		}
		// TODO: do not validate leading and trailing whitespace to preserve backward compatibility.
		// for example: https://github.com/openshift/origin/issues/14659 image = " " is special token in pod template
		// others may have done similar
		if len(ctr.Image) == 0 {
			allErrs = append(allErrs, field.Required(idxPath.Child("image"), ""))
		}
		//if ctr.Lifecycle != nil {
		//	allErrs = append(allErrs, validateLifecycle(ctr.Lifecycle, idxPath.Child("lifecycle"))...)
		//}
		//allErrs = append(allErrs, validateProbe(ctr.LivenessProbe, idxPath.Child("livenessProbe"))...)
		//// Readiness-specific validation
		//if ctr.ReadinessProbe != nil && ctr.ReadinessProbe.TerminationGracePeriodSeconds != nil {
		//	allErrs = append(allErrs, field.Invalid(idxPath.Child("readinessProbe", "terminationGracePeriodSeconds"), ctr.ReadinessProbe.TerminationGracePeriodSeconds, "must not be set for readinessProbes"))
		//}
		//allErrs = append(allErrs, validateProbe(ctr.StartupProbe, idxPath.Child("startupProbe"))...)
		//// Liveness-specific validation
		//if ctr.LivenessProbe != nil && ctr.LivenessProbe.SuccessThreshold != 1 {
		//	allErrs = append(allErrs, field.Invalid(idxPath.Child("livenessProbe", "successThreshold"), ctr.LivenessProbe.SuccessThreshold, "must be 1"))
		//}
		//allErrs = append(allErrs, validateProbe(ctr.StartupProbe, idxPath.Child("startupProbe"))...)
		//// Startup-specific validation
		//if ctr.StartupProbe != nil && ctr.StartupProbe.SuccessThreshold != 1 {
		//	allErrs = append(allErrs, field.Invalid(idxPath.Child("startupProbe", "successThreshold"), ctr.StartupProbe.SuccessThreshold, "must be 1"))
		//}

		switch ctr.TerminationMessagePolicy {
		case v1.TerminationMessageReadFile, v1.TerminationMessageFallbackToLogsOnError:
		case "":
			allErrs = append(allErrs, field.Required(idxPath.Child("terminationMessagePolicy"), "must be 'File' or 'FallbackToLogsOnError'"))
		default:
			allErrs = append(allErrs, field.Invalid(idxPath.Child("terminationMessagePolicy"), ctr.TerminationMessagePolicy, "must be 'File' or 'FallbackToLogsOnError'"))
		}

		//allErrs = append(allErrs, validateProbe(ctr.ReadinessProbe, idxPath.Child("readinessProbe"))...)
		//allErrs = append(allErrs, validateContainerPorts(ctr.Ports, idxPath.Child("ports"))...)
		//allErrs = append(allErrs, ValidateEnv(ctr.Env, idxPath.Child("env"), opts)...)
		//allErrs = append(allErrs, ValidateEnvFrom(ctr.EnvFrom, idxPath.Child("envFrom"))...)
		allErrs = append(allErrs, ValidateVolumeMounts(ctr.VolumeMounts, volDevices, volumes, &ctr, idxPath.Child("volumeMounts"))...)
		allErrs = append(allErrs, ValidateVolumeDevices(ctr.VolumeDevices, volMounts, volumes, idxPath.Child("volumeDevices"))...)
		allErrs = append(allErrs, validatePullPolicy(ctr.ImagePullPolicy, idxPath.Child("imagePullPolicy"))...)
		//allErrs = append(allErrs, ValidateResourceRequirements(&ctr.Resources, idxPath.Child("resources"), opts)...)
		//allErrs = append(allErrs, ValidateSecurityContext(ctr.SecurityContext, idxPath.Child("securityContext"))...)
	}

	//if isInitContainers {
	//	// check initContainers one by one since they are running in sequential order.
	//	for _, initContainer := range containers {
	//		allErrs = append(allErrs, checkHostPortConflicts([]v1.Container{initContainer}, fldPath)...)
	//	}
	//} else {
	//	// Check for colliding ports across all containers.
	//	allErrs = append(allErrs, checkHostPortConflicts(containers, fldPath)...)
	//}

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
