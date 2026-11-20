// Package v1beta1 contains the input type for this Function
// +kubebuilder:object:generate=true
// +groupName=awslookup.fn.exxeon.io
// +versionName=v1beta1
package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// This isn't a custom resource, in the sense that we never install its CRD.
// It is a KRM-like object, so we generate a CRD to describe its schema.

// NotFoundPolicy decides what happens when no AWS resource matches a lookup.
type NotFoundPolicy string

const (
	// NotFoundPolicyFail stops the pipeline with a fatal result. Nothing is
	// created or updated until a matching resource exists.
	NotFoundPolicyFail NotFoundPolicy = "Fail"

	// NotFoundPolicyCreate removes the external name annotation so that the
	// provider creates a new resource.
	NotFoundPolicyCreate NotFoundPolicy = "Create"

	// NotFoundPolicySkip omits an unmatched resource from the desired state.
	// It fails if the managed resource is already observed, to prevent deletion.
	NotFoundPolicySkip NotFoundPolicy = "Skip"
)

// Input configures the AWS lookup Function.
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:categories=crossplane
type Input struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Region to search in when a composed resource has no
	// spec.forProvider.region.
	// +optional
	Region string `json:"region,omitempty"`

	// CredentialsName is the name of the pipeline step credentials that hold
	// AWS credentials. If the step has no credentials with this name, the
	// Function uses the default AWS credential chain, for example EKS Pod
	// Identity or IRSA. Defaults to aws-creds.
	// +optional
	CredentialsName string `json:"credentialsName,omitempty"`

	// AssumeRoleARN is an IAM role to assume before looking up resources.
	// +optional
	AssumeRoleARN string `json:"assumeRoleARN,omitempty"` //nolint:tagliatelle // ARN is an initialism, as in Crossplane providers.

	// OnNotFound decides what happens when no AWS resource matches a lookup.
	// Fail stops the pipeline. Create removes the annotation so that the
	// provider creates a new resource. Skip leaves the resource out of the
	// desired state. Defaults to Fail. A composed resource can override it
	// with the awslookup.fn.exxeon.io/on-not-found annotation.
	// +kubebuilder:validation:Enum=Fail;Create;Skip
	// +optional
	OnNotFound NotFoundPolicy `json:"onNotFound,omitempty"`
}
