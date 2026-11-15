// Package v1beta1 contains the input type for this Function
// +kubebuilder:object:generate=true
// +groupName=awslookup.fn.gideonmaree.dev
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
	// provider creates a new resource. Defaults to Fail.
	// +kubebuilder:validation:Enum=Fail;Create
	// +optional
	OnNotFound NotFoundPolicy `json:"onNotFound,omitempty"`
}
